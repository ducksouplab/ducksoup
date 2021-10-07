package sfu

import (
	"errors"
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

// global state
var (
	mu        sync.Mutex
	roomIndex map[string]*trialRoom
)

func init() {
	mu = sync.Mutex{}
	roomIndex = make(map[string]*trialRoom)
}

// room holds all the resources of a given experiment, accepting an exact number of *size* attendees
type trialRoom struct {
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
	qualifiedId  string
	shortId      string
	namespace    string
	size         int
	duration     int
	neededTracks int
}

func (r *trialRoom) delete() {
	// guard `roomIndex`
	mu.Lock()
	defer mu.Unlock()

	log.Printf("[info] [room#%s] deleting\n", r.shortId)
	delete(roomIndex, r.qualifiedId)
}

// private and not guarded by mutex locks, since called by other guarded methods

func qualifiedId(join types.JoinPayload) string {
	return join.Origin + "#" + join.RoomId
}

func newRoom(qualifiedId string, join types.JoinPayload) *trialRoom {
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

	shortId := join.RoomId

	room := &trialRoom{
		peerServerIndex:     make(map[string]*peerServer),
		filesIndex:          make(map[string][]string),
		connectedIndex:      connectedIndex,
		joinedCountIndex:    joinedCountIndex,
		waitForAllCh:        make(chan struct{}),
		endCh:               make(chan struct{}),
		inTracksReadyCount:  0,
		outTracksReadyCount: 0,
		qualifiedId:         qualifiedId,
		shortId:             shortId,
		namespace:           join.Namespace,
		size:                size,
		duration:            duration,
		neededTracks:        size * TracksPerPeer,
	}
	room.mixer = newMixer(room)
	return room
}

func (r *trialRoom) userCount() int {
	return len(r.connectedIndex)
}

func (r *trialRoom) connectedUserCount() (count int) {
	return len(r.peerServerIndex)
}

func (r *trialRoom) countdown() {
	// blocking "end" event and delete
	endTimer := time.NewTimer(time.Duration(r.duration) * time.Second)
	<-endTimer.C

	for _, ps := range r.peerServerIndex {
		ps.ws.sendWithPayload("end", r.files())
	}

	r.Lock()
	r.running = false
	r.Unlock()

	// listened by peerServers, mixer, localTracks
	close(r.endCh)
	// actual deleting is done when all users have disconnected, see disconnectUser
}

// API read-write

func joinRoom(join types.JoinPayload) (*trialRoom, error) {
	// guard `roomIndex`
	mu.Lock()
	defer mu.Unlock()

	qualifiedId := qualifiedId(join)
	userId := join.UserId

	if r, ok := roomIndex[qualifiedId]; ok {
		r.Lock()
		defer r.Unlock()
		connected, ok := r.connectedIndex[userId]
		if ok {
			// ok -> same user has previously connected
			if connected {
				// user is currently connected (second browser tab or device) -> forbidden
				return nil, errors.New("duplicate")
			} else {
				// reconnects (for instance: page reload)
				r.connectedIndex[userId] = true
				r.joinedCountIndex[userId]++
				return r, nil
			}
		} else if r.userCount() == r.size {
			// room limit reached
			return nil, errors.New("full")
		} else {
			// new user joined existing room
			r.connectedIndex[userId] = true
			r.joinedCountIndex[userId] = 1
			log.Printf("[info] [room#%s] [user#%s] joined\n", join.RoomId, userId)
			return r, nil
		}
	} else {
		log.Printf("[info] [room#%s] [user#%s] created for origin: %s\n", join.RoomId, userId, join.Origin)
		newRoom := newRoom(qualifiedId, join)
		roomIndex[qualifiedId] = newRoom
		return newRoom, nil
	}
}

func (r *trialRoom) incInTracksReadyCount(fromPs *peerServer) {
	r.Lock()
	defer r.Unlock()

	if r.inTracksReadyCount == r.neededTracks {
		// reconnection case, send start
		go fromPs.ws.send("start")
		return
	}

	r.inTracksReadyCount++
	log.Printf("[info] [room#%s] track updated count: %d\n", r.shortId, r.inTracksReadyCount)

	if r.inTracksReadyCount == r.neededTracks {
		log.Printf("[info] [room#%s] users are ready\n", r.shortId)
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

func (r *trialRoom) incOutTracksReadyCount() {
	r.Lock()
	defer r.Unlock()

	r.outTracksReadyCount++

	if r.outTracksReadyCount == r.neededTracks {
		go r.mixer.managedUpdateSignaling("all processed tracks are ready")
	}
}

func (r *trialRoom) decOutTracksReadyCount() {
	r.Lock()
	defer r.Unlock()

	r.outTracksReadyCount--
}

func (r *trialRoom) connectPeerServer(ps *peerServer) {
	r.Lock()
	defer func() {
		r.Unlock()
		r.mixer.managedUpdateSignaling("new user#" + ps.userId)
	}()

	r.peerServerIndex[ps.userId] = ps
}

func (r *trialRoom) disconnectUser(userId string) {
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
			r.delete()
		}
	}
}

func (r *trialRoom) addFiles(userId string, files []string) {
	r.Lock()
	defer r.Unlock()

	r.filesIndex[userId] = append(r.filesIndex[userId], files...)
}

// API read

func (r *trialRoom) joinedCountForUser(userId string) int {
	r.RLock()
	defer r.RUnlock()

	return r.joinedCountIndex[userId]
}

func (r *trialRoom) files() map[string][]string {
	r.RLock()
	defer r.RUnlock()

	return r.filesIndex
}

func (r *trialRoom) endingDelay() (delay int) {
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

func (r *trialRoom) runLocalTrackFromRemote(
	ps *peerServer,
	remoteTrack *webrtc.TrackRemote,
	receiver *webrtc.RTPReceiver,
) {
	outputTrack, err := r.mixer.newLocalTrackFromRemote(ps, remoteTrack, receiver)

	if err != nil {
		log.Printf("[error] [room#%s] runLocalTrackFromRemote: %v\n", r.shortId, err)
	} else {
		// needed to relay control fx events between peer server and output track
		ps := r.peerServerIndex[ps.userId]
		ps.setLocalTrack(remoteTrack.Kind().String(), outputTrack)

		// will trigger signaling if needed
		r.incOutTracksReadyCount()

		outputTrack.loop() // blocking

		// outputTrack has ended
		r.mixer.removeLocalTrack(outputTrack.id)
		r.decOutTracksReadyCount()
	}
}
