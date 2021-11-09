package sfu

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/creamlab/ducksoup/types"
	"github.com/pion/webrtc/v3"
)

const (
	DefaultSize     = 2
	MaxSize         = 8
	TracksPerPeer   = 2
	DefaultDuration = 30
	MaxDuration     = 1200
	Ending          = 10
)

// room holds all the resources of a given experiment, accepting an exact number of *size* attendees
type room struct {
	sync.RWMutex
	// guarded by mutex
	mixer               *mixer
	peerServerIndex     map[string]*peerServer // per user id
	connectedIndex      map[string]bool        // per user id, undefined: never connected, false: previously connected, true: connected
	joinedCountIndex    map[string]int         // per user id
	filesIndex          map[string][]string    // per user id, contains media file names
	running             bool
	startedAt           time.Time
	inTracksReadyCount  int
	outTracksReadyCount int
	// channels (safe)
	waitForAllCh chan struct{}
	endCh        chan struct{}
	// other (written only during initialization)
	id           string
	qualifiedId  string // prefixed by origin, used for indexing in roomStore
	namespace    string
	size         int
	duration     int
	neededTracks int
}

// private and not guarded by mutex locks, since called by other guarded methods

func qualifiedId(join types.JoinPayload) string {
	return join.Origin + "#" + join.RoomId
}

func newRoom(qualifiedId string, join types.JoinPayload) *room {
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

	// room initialized with one connected peer
	connectedIndex := make(map[string]bool)
	connectedIndex[join.UserId] = true
	joinedCountIndex := make(map[string]int)
	joinedCountIndex[join.UserId] = 1

	// create folder for logs
	helpers.EnsureDir("./data/" + join.Namespace)
	helpers.EnsureDir("./data/" + join.Namespace + "/logs") // used by x264 mutipass cache

	r := &room{
		peerServerIndex:     make(map[string]*peerServer),
		filesIndex:          make(map[string][]string),
		connectedIndex:      connectedIndex,
		joinedCountIndex:    joinedCountIndex,
		waitForAllCh:        make(chan struct{}),
		endCh:               make(chan struct{}),
		inTracksReadyCount:  0,
		outTracksReadyCount: 0,
		qualifiedId:         qualifiedId,
		id:                  join.RoomId,
		namespace:           join.Namespace,
		size:                size,
		duration:            duration,
		neededTracks:        size * TracksPerPeer,
	}
	r.mixer = newMixer(r)
	return r
}

func (r *room) userCount() int {
	return len(r.connectedIndex)
}

func (r *room) connectedUserCount() (count int) {
	return len(r.peerServerIndex)
}

func (r *room) filePrefixWithCount(join types.JoinPayload) string {
	connectionCount := r.joinedCountForUser(join.UserId)
	// time room user count
	return time.Now().Format("20060102-150405.000") +
		"-r-" + join.RoomId +
		"-u-" + join.UserId +
		"-c-" + fmt.Sprint(connectionCount)
}

func (r *room) countdown() {
	// blocking "end" event and delete
	endTimer := time.NewTimer(time.Duration(r.duration) * time.Second)
	<-endTimer.C

	for _, ps := range r.peerServerIndex {
		ps.ws.sendWithPayload("end", r.files())
	}

	r.Lock()
	r.running = false
	r.Unlock()

	// listened by peerServers, mixer, mixerTracks
	close(r.endCh)
	// actual deleting is done when all users have disconnected, see disconnectUser
}

// API read-write

func (r *room) incInTracksReadyCount(fromPs *peerServer) {
	r.Lock()
	defer r.Unlock()

	if r.inTracksReadyCount == r.neededTracks {
		// reconnection case, send start
		go fromPs.ws.send("start")
		return
	}

	r.inTracksReadyCount++
	log.Printf("[info] [room#%s] track updated count: %d\n", r.id, r.inTracksReadyCount)

	if r.inTracksReadyCount == r.neededTracks {
		log.Printf("[info] [room#%s] users are ready\n", r.id)
		close(r.waitForAllCh)
		r.running = true
		r.startedAt = time.Now()
		// send start to all peers
		for _, ps := range r.peerServerIndex {
			go ps.ws.send("start")
		}
		go r.countdown()
		return
	}
}

func (r *room) incOutTracksReadyCount() {
	r.Lock()
	defer r.Unlock()

	r.outTracksReadyCount++

	if r.outTracksReadyCount == r.neededTracks {
		// TOFIX without this timeout, some tracks are not sent to peers,
		<-time.After(1000 * time.Millisecond)
		go r.mixer.managedUpdateSignaling("all processed tracks are ready")
	}
}

func (r *room) decOutTracksReadyCount() {
	r.Lock()
	defer r.Unlock()

	r.outTracksReadyCount--
}

func (r *room) connectPeerServer(ps *peerServer) {
	r.Lock()
	defer func() {
		r.Unlock()
		r.mixer.managedUpdateSignaling("new user#" + ps.userId)
	}()

	r.peerServerIndex[ps.userId] = ps
}

func (r *room) disconnectUser(userId string) {
	r.Lock()
	defer r.Unlock()

	// protects decrementing since RemovePeer maybe called several times for same user
	if r.connectedIndex[userId] {
		// remove user current connection details (=peerServer)
		delete(r.peerServerIndex, userId)
		// mark disconnected, but keep track of her
		r.connectedIndex[userId] = false
		go r.mixer.managedUpdateSignaling("disconnected")

		if r.connectedUserCount() == 0 && !r.running {
			// don't keep this room
			rooms.delete(r)
		}
	}
}

func (r *room) addFiles(userId string, files []string) {
	r.Lock()
	defer r.Unlock()

	r.filesIndex[userId] = append(r.filesIndex[userId], files...)
}

// API read

func (r *room) joinedCountForUser(userId string) int {
	r.RLock()
	defer r.RUnlock()

	return r.joinedCountIndex[userId]
}

func (r *room) files() map[string][]string {
	r.RLock()
	defer r.RUnlock()

	return r.filesIndex
}

func (r *room) endingDelay() (delay int) {
	r.RLock()
	defer r.RUnlock()

	elapsed := time.Since(r.startedAt)

	remaining := r.duration - int(elapsed.Seconds())
	delay = remaining - Ending
	if delay < 1 {
		delay = 1
	}
	return
}

func (r *room) readRemoteWhileWaiting(remoteTrack *webrtc.TrackRemote) {
	for {
		select {
		case <-r.waitForAllCh:
			// trial is over, no need to trigger signaling on every closing track
			return
		default:
			_, _, err := remoteTrack.ReadRTP()
			if err != nil {
				log.Printf("[error] [room#%s] readRemoteWhileWaiting: %v\n", r.id, err)
				return
			}
		}
	}
}

func (r *room) runMixerSliceFromRemote(
	ps *peerServer,
	remoteTrack *webrtc.TrackRemote,
	receiver *webrtc.RTPReceiver,
) {
	// signal new peer and tracks
	r.incInTracksReadyCount(ps)

	// wait for all peers to connect
	r.readRemoteWhileWaiting(remoteTrack)

	slice, err := r.mixer.newMixerSliceFromRemote(ps, remoteTrack, receiver)

	if err != nil {
		log.Printf("[error] [room#%s] runMixerSliceFromRemote: %v\n", r.id, err)
	} else {
		// needed to relay control fx events between peer server and output track
		ps.setMixerSlice(remoteTrack.Kind().String(), slice)

		// will trigger signaling if needed
		r.incOutTracksReadyCount()

		slice.loop() // blocking

		// track has ended
		r.mixer.removeMixerSlice(slice)
		r.decOutTracksReadyCount()
	}
}
