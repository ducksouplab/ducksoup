package sfu

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/config"
	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/ducksouplab/ducksoup/store"
	"github.com/ducksouplab/ducksoup/types"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	DefaultSize              = 2
	MaxSize                  = 8
	DefaultDurationInSeconds = 30
	MaxDurationInSeconds     = 1200
	AbortLimitInSeconds      = 15
	EndingInSeconds          = 15
)

// interaction holds all the resources of a given experiment, accepting an exact number of *size* attendees
type interaction struct {
	sync.RWMutex
	// guarded by mutex
	mixer               *mixer
	peerServerIndex     map[string]*peerServer // per user id
	connectedIndex      map[string]bool        // per user id, undefined: never connected, false: previously connected, true: connected
	joinedCountIndex    map[string]int         // per user id
	filesIndex          map[string][]string    // per user id, contains media file names
	ready               bool                   // all in tracks are there
	started             bool                   // changed once to show if interaction has been aborted or not
	deleted             bool
	createdAt           time.Time
	startedAt           time.Time
	pipelineStartCount  int
	inTracksReadyCount  int
	outTracksReadyCount int
	// channels (safe)
	readyCh   chan struct{}
	startedCh chan struct{}
	abortedCh chan struct{}
	doneCh    chan struct{}
	// other (written only during initialization)
	id           string // origin+namespace+name, used for indexing in interactionStore
	randomId     string // random internal id
	namespace    string
	name         string // public name
	size         int
	duration     time.Duration
	neededTracks int
	ssrcs        []uint32
	jp           types.JoinPayload
	dataFolder   string
	// log
	logger zerolog.Logger
	// internals
	abortTimer    *time.Timer
	gracefulTimer *time.Timer
}

type userStream struct {
	UserId   string `json:"userId"`
	StreamId string `json:"streamId"`
}

// private and not guarded by mutex locks, since called by other guarded methods

func generateId(jp types.JoinPayload) string {
	return jp.Origin + "#" + jp.Namespace + "#" + jp.InteractionName
}

func (i *interaction) setLogger() {
	var logger zerolog.Logger

	path := i.DataFolder() + "/" + i.name + "-a-" + time.Now().Format("20060102-150405.000") + ".log"
	fileWriter, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)

	if err == nil {
		writers := zerolog.MultiLevelWriter(fileWriter, zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: env.TimeFormat})
		logger = zerolog.New(writers).With().Timestamp().Logger()
	} else {
		// use default logger
		logger = log.Logger
	}
	// add default options
	logger = logger.With().
		Str("namespace", i.namespace).
		Str("interaction", i.name).
		Logger().
		Hook(i) // (call Run hook whenever logging)
	// DEV mode
	if env.Mode == "DEV" {
		logger = logger.With().Caller().Logger()
	}
	// first log
	logger.Info().Str("context", "interaction").Msg("logger_created")

	i.logger = logger
}

func newInteraction(id string, jp types.JoinPayload) *interaction {
	// process duration
	durationInSeconds := jp.Duration
	if durationInSeconds < 1 {
		durationInSeconds = DefaultDurationInSeconds
	} else if durationInSeconds > MaxDurationInSeconds {
		durationInSeconds = MaxDurationInSeconds
	}

	// process size
	size := jp.Size
	if size < 1 {
		size = DefaultSize
	} else if size > MaxSize {
		size = MaxSize
	}
	neededTracks := size * 2 // 1 audio and 1 video track per peer
	if jp.AudioOnly {
		neededTracks = size // 1 audio track per peer
	}

	// interaction initialized with one connected peer
	connectedIndex := make(map[string]bool)
	connectedIndex[jp.UserId] = true
	joinedCountIndex := make(map[string]int)
	joinedCountIndex[jp.UserId] = 1

	i := &interaction{
		peerServerIndex:     make(map[string]*peerServer),
		filesIndex:          make(map[string][]string),
		deleted:             false,
		connectedIndex:      connectedIndex,
		joinedCountIndex:    joinedCountIndex,
		readyCh:             make(chan struct{}),
		startedCh:           make(chan struct{}),
		abortedCh:           make(chan struct{}),
		doneCh:              make(chan struct{}),
		createdAt:           time.Now(),
		pipelineStartCount:  0,
		inTracksReadyCount:  0,
		outTracksReadyCount: 0,
		randomId:            helpers.RandomHexString(12),
		namespace:           jp.Namespace,
		id:                  id,
		name:                jp.InteractionName,
		size:                size,
		duration:            time.Duration(durationInSeconds) * time.Second,
		neededTracks:        neededTracks,
		ssrcs:               []uint32{},
		jp:                  jp,
		dataFolder:          fmt.Sprintf("data/%v/%v", jp.Namespace, jp.InteractionName),
		abortTimer:          time.NewTimer(time.Duration(AbortLimitInSeconds) * time.Second),
	}
	// create data folders
	helpers.EnsureDir("./" + i.dataFolder + "/recordings")
	if env.GeneratePlots {
		helpers.EnsureDir("./" + i.dataFolder + "/plots")
	}
	if jp.VideoFormat == "H264" {
		// used by x264 mutipass cache or muxer
		helpers.EnsureDir("./" + i.dataFolder + "/cache")
	}
	i.mixer = newMixer(i)
	i.setLogger()

	i.logger.Info().Str("context", "interaction").Str("user", jp.UserId).Str("origin", jp.Origin).Msg("interaction_created")
	i.logger.Info().Str("context", "interaction").Str("user", jp.UserId).Interface("payload", jp).Msg("peer_joined")

	go i.abortCountdown()
	return i
}

func (i *interaction) DataFolder() string {
	return i.dataFolder
}

// Run: implement log Hook interface
func (i *interaction) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	sinceCreation := time.Since(i.createdAt).Milliseconds()
	e.Str("sinceCreation", fmt.Sprintf("%vms", sinceCreation))
	if i.started {
		sinceStart := time.Since(i.startedAt).Milliseconds()
		e.Str("sinceStart", fmt.Sprintf("%vms", sinceStart))
	}
}

func (i *interaction) isReady() chan struct{} {
	return i.readyCh
}

func (i *interaction) isStarted() chan struct{} {
	return i.startedCh
}

func (i *interaction) isAborted() chan struct{} {
	return i.abortedCh
}

func (i *interaction) isDone() chan struct{} {
	return i.doneCh
}

func (i *interaction) join(jp types.JoinPayload) (msg string, err error) {
	i.Lock()
	defer i.Unlock()

	userId := jp.UserId
	connected, ok := i.connectedIndex[userId]
	if ok {
		// ok -> same user has previously connected
		if connected {
			// user is currently connected (second browser tab or device) -> forbidden
			return "error", errors.New("duplicate")
		} else {
			// reconnects (for instance: page reload)
			i.connectedIndex[userId] = true
			i.joinedCountIndex[userId]++
			return "reconnection", nil
		}
	} else if len(i.connectedIndex) == i.size { // length of users that have connected (even if aren't still)
		// interaction limit reached
		return "error", errors.New("full")
	} else {
		// new user joined existing interaction: normal path
		i.connectedIndex[userId] = true
		i.joinedCountIndex[userId] = 1
		i.logger.Info().Str("context", "interaction").Str("user", userId).Interface("payload", jp).Msg("peer_joined")
		return "existing-interaction", nil
	}
}

func (i *interaction) unguardedConnectedUserCount() (count int) {
	// don't rely on i.peerServerIndex, which is useful only after i.connectPeerServer(ps)
	// then prefer connectedIndex

	for _, isConnected := range i.connectedIndex {
		if isConnected {
			count++
		}
	}
	return
}

// users are connected but some out tracks may still be in the process of
// being attached and (not) ready (yet)
func (i *interaction) allUsersConnected() bool {
	i.RLock()
	defer i.RUnlock()

	return i.unguardedConnectedUserCount() == i.size
}

// Called when one of the pipeline of the interaction is ready
// (can be called several times if there are several users,
// but only the first will have an effect)
// Waiting for a pipeline to be started provides more accurates
// (relatively to client-side experience) startedAt and gracefulCountdown
func (i *interaction) start() {
	i.Lock()
	defer i.Unlock()

	i.pipelineStartCount++

	// starts if: not already started
	// AND either size is 1 (participant) or at leat two pipeline have started (for size >= 2)
	if !i.started && (i.size == 1 || i.pipelineStartCount > 1) {
		i.started = true
		i.startedAt = time.Now()
		i.logger.Info().Str("context", "interaction").Msg("interaction_started")
		// send start to all peers
		for _, ps := range i.peerServerIndex {
			go ps.ws.sendWithPayload("start", i.remainingSeconds())
		}
		go i.gracefulCountdown()
		close(i.startedCh)
	}
}

func (i *interaction) stop(graceful bool) {
	// listened by peerServers, mixer, mixerTracks
	if graceful {
		close(i.doneCh)
		i.logger.Info().Str("context", "interaction").Msg("interaction_end")
	} else {
		close(i.abortedCh)
		i.logger.Info().Str("context", "interaction").Msg("interaction_aborted")
	}
	i.ready = false

	<-time.After(3000 * time.Millisecond)
	// most likely already deleted, see disconnectUser
	// except if interaction was empty before turning to i.allInTracksReady=false
	i.unguardedDelete()
}

// ends room if not enough user have connected after a waiting limit
func (i *interaction) abortCountdown() {
	// wait then check if allInTracksReady, if not, abort
	<-i.abortTimer.C

	i.RLock()
	if !i.ready {
		i.RUnlock()
		i.stop(false)
	}
}

// ends room when its duration has been reached
func (i *interaction) gracefulCountdown() {
	// blocking "end" event and delete
	i.gracefulTimer = time.NewTimer(i.duration)
	<-i.gracefulTimer.C
	i.logger.Info().Str("context", "interaction").Msg("graceful_countdown_reached")

	i.stop(true)
}

// API read-write

func (i *interaction) incInTracksReadyCount(fromPs *peerServer, remoteTrack *webrtc.TrackRemote) {
	i.Lock()
	isAlreadyReady := i.inTracksReadyCount == i.neededTracks
	i.Unlock()

	if isAlreadyReady {
		// reconnection case, then send start only once
		// test on audio not to send it twice and since there is always an audio track
		if remoteTrack.Kind().String() == "audio" {
			go fromPs.ws.sendWithPayload("start", i.remainingSeconds())
		}
		return
	}

	i.Lock()
	i.inTracksReadyCount++
	i.logger.Info().Str("context", "interaction").Int("count", i.inTracksReadyCount).Msg("in_track_added_to_interaction")
	isReadyNow := i.inTracksReadyCount == i.neededTracks
	if isReadyNow {
		i.ready = true
		i.abortTimer.Stop()
		close(i.readyCh)
	}
	i.Unlock()

}

func (i *interaction) addSSRC(ssrc uint32, kind string, userId string) {
	i.Lock()
	defer i.Unlock()

	i.ssrcs = append(i.ssrcs, ssrc)
	go store.AddToSSRCIndex(ssrc, kind, i.namespace, i.name, userId, i.logger)
}

// returns true if signaling is needed
func (i *interaction) incOutTracksReadyCount() bool {
	i.Lock()
	defer i.Unlock()

	i.outTracksReadyCount++

	// all out tracks are ready
	if i.outTracksReadyCount == i.neededTracks {
		return true
	}
	// interaction with >= 3 users: after two users have disconnected and only one came back
	// trigger signaling (for an even number of out tracks since they go
	// by audio/video pairs)
	if i.ready && (i.outTracksReadyCount%2 == 0) {
		return true
	}
	return false
}

func (i *interaction) decOutTracksReadyCount() {
	i.Lock()
	defer i.Unlock()

	i.outTracksReadyCount--
}

func (i *interaction) connectPeerServer(ps *peerServer) {
	i.Lock()
	defer i.Unlock()

	// advertise everyone of each other
	for _, other := range i.peerServerIndex {
		go other.ws.sendWithPayload("other_joined", userStream{
			ps.userId,
			ps.streamId,
		})
		go ps.ws.sendWithPayload("other_joined", userStream{
			other.userId,
			other.streamId,
		})
	}
	// add peer
	i.peerServerIndex[ps.userId] = ps
}

// should be called by another method that locked the interaction (mutex)
func (i *interaction) unguardedDelete() {
	if i.deleted {
		return
	}
	i.deleted = true
	interactionStoreSingleton.delete(i)
	i.logger.Info().Str("context", "interaction").Msg("interaction_deleted")
	// cleanup
	for _, ssrc := range i.ssrcs {
		store.RemoveFromSSRCIndex(ssrc)
	}
}

func (i *interaction) disconnectUser(ps *peerServer) {
	i.Lock()
	defer i.Unlock()

	// protects decrementing since RemovePeer maybe called several times for same user
	if i.connectedIndex[ps.userId] {
		// remove user current connection details (=peerServer)
		delete(i.peerServerIndex, ps.userId)
		// advertise others
		for _, other := range i.peerServerIndex {
			go other.ws.sendWithPayload("other_left", userStream{
				ps.userId,
				ps.streamId,
			})
		}
		// mark disconnected, but keep track of her
		i.connectedIndex[ps.userId] = false

		// prevent useless signaling when aborting/ending room
		if i.deleted {
			go i.mixer.managedSignalingForEveryone("user_disconnected", false)
		}

		// users may have disconnected temporarily
		// delete only if is empty and not running
		if i.unguardedConnectedUserCount() == 0 && !i.ready && !i.deleted {
			i.abortTimer.Stop()
			i.unguardedDelete()
		}
	}
}

func (i *interaction) addFiles(userId string, files []string) {
	i.Lock()
	defer i.Unlock()

	i.filesIndex[userId] = append(i.filesIndex[userId], files...)
}

// API read

func (i *interaction) joinedCountForUser(userId string) int {
	i.RLock()
	defer i.RUnlock()

	return i.joinedCountIndex[userId]
}

func (i *interaction) files() map[string][]string {
	i.RLock()
	defer i.RUnlock()

	return i.filesIndex
}

func (i *interaction) remainingSeconds() int {
	elapsed := time.Since(i.startedAt)
	return int(i.duration.Seconds() - elapsed.Seconds())
}

func (i *interaction) endingDelay() (delay int) {
	i.RLock()
	defer i.RUnlock()

	elapsed := time.Since(i.startedAt)

	remaining := int(i.duration.Seconds() - elapsed.Seconds())
	delay = remaining - EndingInSeconds
	if delay < 1 {
		delay = 1
	}
	return
}

// return false if an error ends the waiting, discards RTP till ready
func (i *interaction) loopTillAllReady(remoteTrack *webrtc.TrackRemote) bool {
	buf := make([]byte, config.SFU.Common.MTU)
	for {
		select {
		case <-i.isReady():
			return true
		case <-i.isAborted():
			return false
		default:
			_, _, err := remoteTrack.Read(buf)
			if err != nil {
				i.logger.Error().Str("context", "track").Err(err).Msg("loop_till_all_ready_failed")
				return false
			}
		}
	}
}

func (i *interaction) runMixerSliceFromRemote(
	ps *peerServer,
	remoteTrack *webrtc.TrackRemote,
	receiver *webrtc.RTPReceiver,
) {
	// signal new peer and tracks
	i.incInTracksReadyCount(ps, remoteTrack)

	// prepare slice
	slice, err := newMixerSlice(ps, remoteTrack, receiver)
	if err == nil {
		// index to be searchable by track id
		i.mixer.indexMixerSlice(slice)
		// needed to relay control fx events between peer server and output track
		ps.setMixerSlice(remoteTrack.Kind().String(), slice)

		// wait for all peers to connect
		ok := i.loopTillAllReady(remoteTrack)

		if ok {
			// trigger signaling if needed
			signalingNeeded := i.incOutTracksReadyCount()
			if signalingNeeded {
				// TODO FIX WITH CAUTION: without this timeout, some tracks are not sent to peers
				<-time.After(100 * time.Millisecond)
				go i.mixer.managedSignalingForEveryone("out_tracks_ready", true)
			}
			// blocking until interaction ends or user disconnects
			slice.loop()
			// track has ended
			i.mixer.removeMixerSlice(slice)
			i.decOutTracksReadyCount()
		}
	}
}
