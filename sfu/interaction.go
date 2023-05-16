package sfu

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/ducksouplab/ducksoup/store"
	"github.com/ducksouplab/ducksoup/types"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	DefaultSize     = 2
	MaxSize         = 8
	TracksPerPeer   = 2
	DefaultDuration = 30
	MaxDuration     = 1200
	AbortLimit      = 10
	Ending          = 15
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
	running             bool
	started             bool // changed once to show if interaction has been aborted or not
	deleted             bool
	createdAt           time.Time
	startedAt           time.Time
	inTracksReadyCount  int
	outTracksReadyCount int
	// channels (safe)
	readyCh   chan struct{}
	abortedCh chan struct{}
	doneCh    chan struct{}
	// other (written only during initialization)
	id           string // origin+namespace+name, used for indexing in interactionStore
	randomId     string // random internal id
	namespace    string
	name         string // public name
	size         int
	duration     int
	neededTracks int
	ssrcs        []uint32
	// log
	logger zerolog.Logger
	// internals
	abortTimer    *time.Timer
	gracefulTimer *time.Timer
}

// private and not guarded by mutex locks, since called by other guarded methods

func generateId(join types.JoinPayload) string {
	return join.Origin + "#" + join.Namespace + "#" + join.InteractionName
}

func newInteraction(id string, join types.JoinPayload) *interaction {
	// process duration
	duration := join.Duration
	if duration < 1 {
		duration = DefaultDuration
	} else if duration > MaxDuration {
		duration = MaxDuration
	}

	// process size
	size := join.Size
	if size < 1 {
		size = DefaultSize
	} else if size > MaxSize {
		size = MaxSize
	}

	// interaction initialized with one connected peer
	connectedIndex := make(map[string]bool)
	connectedIndex[join.UserId] = true
	joinedCountIndex := make(map[string]int)
	joinedCountIndex[join.UserId] = 1

	// create folder for logs
	helpers.EnsureDir("./data/" + join.Namespace)
	helpers.EnsureDir("./data/" + join.Namespace + "/logs") // used by x264 mutipass cache

	i := &interaction{
		peerServerIndex:     make(map[string]*peerServer),
		filesIndex:          make(map[string][]string),
		deleted:             false,
		connectedIndex:      connectedIndex,
		joinedCountIndex:    joinedCountIndex,
		readyCh:             make(chan struct{}),
		abortedCh:           make(chan struct{}),
		doneCh:              make(chan struct{}),
		createdAt:           time.Now(),
		inTracksReadyCount:  0,
		outTracksReadyCount: 0,
		randomId:            helpers.RandomHexString(12),
		namespace:           join.Namespace,
		id:                  id,
		name:                join.InteractionName,
		size:                size,
		duration:            duration,
		neededTracks:        size * TracksPerPeer,
		ssrcs:               []uint32{},
		abortTimer:          time.NewTimer(time.Duration(AbortLimit) * time.Second),
	}
	i.mixer = newMixer(i)
	// log (call Run hook whenever logging)
	i.logger = log.With().
		Str("context", "interaction").
		Str("namespace", join.Namespace).
		Str("interaction", join.InteractionName).
		Logger().
		Hook(i)

	go i.abortCountdown()
	return i
}

func (i *interaction) ready() chan struct{} {
	return i.readyCh
}

func (i *interaction) aborted() chan struct{} {
	return i.abortedCh
}

func (i *interaction) done() chan struct{} {
	return i.doneCh
}

// Run: implement log Hook interface
func (i *interaction) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	sinceCreation := time.Since(i.createdAt).Round(time.Millisecond).String()
	e.Str("sinceCreation", sinceCreation)
	if i.started {
		sinceStart := time.Since(i.startedAt).Round(time.Millisecond).String()
		e.Str("sinceStart", sinceStart)
	}
}

func (i *interaction) join(join types.JoinPayload) (msg string, err error) {
	i.Lock()
	defer i.Unlock()

	userId := join.UserId
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
		// new user joined existing interaction: normal path! (2)
		i.connectedIndex[userId] = true
		i.joinedCountIndex[userId] = 1
		log.Info().Str("context", "interaction").Str("namespace", join.Namespace).Str("interaction", join.InteractionName).Str("user", userId).Interface("payload", join).Msg("peer_joined")
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
	i.Lock()
	defer i.Unlock()

	return i.unguardedConnectedUserCount() == i.size
}

func (i *interaction) filePrefix(userId string) string {
	connectionCount := i.joinedCountForUser(userId)
	// caution: time reflects the moment the pipeline is initialized.
	// When pipeline is started, files are written to, but it's better
	// to rely on the time advertised by the OS (file properties)
	// if several files need to be synchronized
	return "i-" + i.randomId +
		"-a-" + time.Now().Format("20060102-150405.000") +
		"-s-" + i.namespace +
		"-n-" + i.name +
		"-u-" + userId +
		"-c-" + fmt.Sprint(connectionCount)
}

func (i *interaction) start() {
	i.Lock()
	defer i.Unlock()
	// do start
	i.abortTimer.Stop()
	close(i.readyCh)
	i.started = true
	i.running = true
	i.logger.Info().Msg("interaction_started")
	i.startedAt = time.Now()
	// send start to all peers
	for _, ps := range i.peerServerIndex {
		go ps.ws.send("start")
	}
	go i.gracefulCountdown()
}

func (i *interaction) stop(graceful bool) {
	// listened by peerServers, mixer, mixerTracks
	close(i.doneCh)
	if graceful {
		i.logger.Info().Msg("interaction_end")
	} else {
		i.logger.Info().Msg("interaction_aborted")
	}
	i.running = false

	<-time.After(3000 * time.Millisecond)
	// most likely already deleted, see disconnectUser
	// except if interaction was empty before turning to i.running=false
	i.unguardedDelete()
}

// ends room if not enough user have connected after a waiting limit
func (i *interaction) abortCountdown() {
	// wait then check if interaction is running, if not, abort
	<-i.abortTimer.C

	i.Lock()
	if !i.running {
		i.stop(false)
	}
	i.Unlock()
}

// ends room when its duration has been reached
func (i *interaction) gracefulCountdown() {
	// blocking "end" event and delete
	i.gracefulTimer = time.NewTimer(time.Duration(i.duration) * time.Second)
	<-i.gracefulTimer.C

	i.Lock()
	i.stop(true)
	i.Unlock()
}

// API read-write

func (i *interaction) incInTracksReadyCount(fromPs *peerServer, remoteTrack *webrtc.TrackRemote) {
	i.Lock()
	isAlreadyReady := i.inTracksReadyCount == i.neededTracks
	i.Unlock()

	if isAlreadyReady {
		// reconnection case, then send start only once (check for "audio" for instance)
		if remoteTrack.Kind().String() == "audio" {
			go fromPs.ws.send("start")
		}
		return
	}

	i.Lock()
	i.inTracksReadyCount++
	i.logger.Info().Int("count", i.inTracksReadyCount).Msg("in_track_added_to_interaction")
	isReadyNow := i.inTracksReadyCount == i.neededTracks
	i.Unlock()

	if isReadyNow {
		i.start()
	}
}

func (i *interaction) addSSRC(ssrc uint32, kind string, userId string) {
	i.Lock()
	defer i.Unlock()

	i.ssrcs = append(i.ssrcs, ssrc)
	go store.AddToSSRCIndex(ssrc, kind, i.namespace, i.name, userId)
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
	if i.running && (i.outTracksReadyCount%2 == 0) {
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

	i.peerServerIndex[ps.userId] = ps
}

// should be called by another method that locked the interaction (mutex)
func (i *interaction) unguardedDelete() {
	if i.deleted {
		return
	}
	i.deleted = true
	interactionStoreSingleton.delete(i)
	i.logger.Info().Msg("interaction_deleted")
	// cleanup
	for _, ssrc := range i.ssrcs {
		store.RemoveFromSSRCIndex(ssrc)
	}
}

func (i *interaction) disconnectUser(userId string) {
	i.Lock()
	defer i.Unlock()

	// protects decrementing since RemovePeer maybe called several times for same user
	if i.connectedIndex[userId] {
		// remove user current connection details (=peerServer)
		delete(i.peerServerIndex, userId)
		// mark disconnected, but keep track of her
		i.connectedIndex[userId] = false

		// prevent useless signaling when aborting/ending room
		if i.deleted {
			go i.mixer.managedSignalingForEveryone("user_disconnected", false)
		}

		// users may have disconnected temporarily
		// delete only if is empty and not running
		if i.unguardedConnectedUserCount() == 0 && !i.running && !i.deleted {
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

func (i *interaction) endingDelay() (delay int) {
	i.RLock()
	defer i.RUnlock()

	elapsed := time.Since(i.startedAt)

	remaining := i.duration - int(elapsed.Seconds())
	delay = remaining - Ending
	if delay < 1 {
		delay = 1
	}
	return
}

// return false if an error ends the waiting, discards RTP till ready
func (i *interaction) loopTillAllReady(remoteTrack *webrtc.TrackRemote) bool {
	for {
		select {
		case <-i.ready():
			return true
		case <-i.aborted():
			return false
		default:
			_, _, err := remoteTrack.ReadRTP()
			if err != nil {
				i.logger.Error().Err(err).Msg("loop_till_all_ready_failed")
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
	if err != nil {
		i.logger.Error().Err(err).Msg("new_mixer_slice_failed")
	}
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
			<-time.After(2000 * time.Millisecond)
			go i.mixer.managedSignalingForEveryone("out_tracks_ready", true)
		}
		// blocking until interaction ends or user disconnects
		slice.loop()
		// track has ended
		i.mixer.removeMixerSlice(slice)
		i.decOutTracksReadyCount()
	}
}
