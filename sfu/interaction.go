package sfu

import (
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
	deleted             bool
	createdAt           time.Time
	startedAt           time.Time
	inTracksReadyCount  int
	outTracksReadyCount int
	// channels (safe)
	waitForAllCh chan struct{}
	endCh        chan struct{}
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
		waitForAllCh:        make(chan struct{}),
		endCh:               make(chan struct{}),
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
	}
	i.mixer = newMixer(i)
	// log (call Run hook whenever logging)
	i.logger = log.With().
		Str("context", "interaction").
		Str("namespace", join.Namespace).
		Str("interaction", join.InteractionName).
		Logger().
		Hook(i)

	return i
}

// Run: implement log Hook interface
func (i *interaction) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	sinceCreation := time.Since(i.createdAt).Round(time.Millisecond).String()
	e.Str("sinceCreation", sinceCreation)
	if !i.startedAt.IsZero() {
		sinceStart := time.Since(i.startedAt).Round(time.Millisecond).String()
		e.Str("sinceStart", sinceStart)
	}
}

func (i *interaction) userCount() int {
	return len(i.connectedIndex)
}

func (i *interaction) connectedUserCount() (count int) {
	return len(i.peerServerIndex)
}

// users are connected but some out tracks may still be in the process of
// being attached and (not) ready (yet)
func (i *interaction) allUsersConnected() bool {
	return i.connectedUserCount() == i.size
}

func (i *interaction) filePrefix(userId string) string {
	connectionCount := i.joinedCountForUser(userId)
	// caution: time reflects the moment the pipeline is initialized.
	// When pipeline is started, files are written to, but it's better
	// to rely on the time advertised by the OS (file properties)
	// if several files need to be synchronized
	return "i-" + i.randomId +
		"-a-" + time.Now().Format("20060102-150405.000") +
		"-ns-" + i.namespace +
		"-n-" + i.name +
		"-u-" + userId +
		"-c-" + fmt.Sprint(connectionCount)
}

func (i *interaction) countdown() {
	// blocking "end" event and delete
	endTimer := time.NewTimer(time.Duration(i.duration) * time.Second)
	<-endTimer.C

	i.Lock()
	i.running = false
	i.Unlock()

	i.logger.Info().Msg("interaction_ended")
	// listened by peerServers, mixer, mixerTracks
	close(i.endCh)

	<-time.After(3000 * time.Millisecond)
	// most likely already deleted, see disconnectUser
	// except if interaction was empty before turning to i.running=false
	i.Lock()
	if !i.deleted {
		i.unguardedDelete()
	}
	i.Unlock()
}

// API read-write

func (i *interaction) incInTracksReadyCount(fromPs *peerServer, remoteTrack *webrtc.TrackRemote) {
	i.Lock()
	defer i.Unlock()

	if i.inTracksReadyCount == i.neededTracks {
		// reconnection case, then send start only once (check for "audio" for instance)
		if remoteTrack.Kind().String() == "audio" {
			go fromPs.ws.send("start")
		}
		return
	}

	i.inTracksReadyCount++
	i.logger.Info().Int("count", i.inTracksReadyCount).Msg("in_track_added_to_interaction")

	if i.inTracksReadyCount == i.neededTracks {
		// do start
		close(i.waitForAllCh)
		i.running = true
		i.logger.Info().Msg("interaction_started")
		i.startedAt = time.Now()
		// send start to all peers
		for _, ps := range i.peerServerIndex {
			go ps.ws.send("start")
		}
		go i.countdown()
		return
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
		go i.mixer.managedGlobalSignaling("user_disconnected", false)

		// users may have disconnected temporarily
		// delete only if is empty and not running
		if i.connectedUserCount() == 0 && !i.running && !i.deleted {
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

// return false if an error ends the waiting
func (i *interaction) readRemoteTillAllReady(remoteTrack *webrtc.TrackRemote) bool {
	for {
		select {
		case <-i.waitForAllCh:
			// trial is over, no need to trigger signaling on every closing track
			return true
		default:
			_, _, err := remoteTrack.ReadRTP()
			if err != nil {
				i.logger.Error().Err(err).Msg("read_remote_till_all_ready_failed")
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
	ok := i.readRemoteTillAllReady(remoteTrack)

	if ok {
		// trigger signaling if needed
		signalingNeeded := i.incOutTracksReadyCount()
		if signalingNeeded {
			// TODO FIX, CAUTION without this timeout, some tracks are not sent to peers,
			<-time.After(500 * time.Millisecond)
			go i.mixer.managedGlobalSignaling("out_tracks_ready", true)
		}
		// blocking until interaction ends or user disconnects
		slice.loop()
		// track has ended
		i.mixer.removeMixerSlice(slice)
		i.decOutTracksReadyCount()
	}
}