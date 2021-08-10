package sfu

import (
	"errors"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/pion/webrtc/v3"
)

const (
	DefaultSize        = 2
	MaxSize            = 8
	TracksPerPeer      = 2
	DefaultDuration    = 30
	MaxDuration        = 1200
	Ending             = 10
	MaxNamespaceLength = 30
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
	started             bool
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

	log.Printf("[room %s] deleted\n", r.shortId)
	delete(roomIndex, r.qualifiedId)
}

// remove special characters like / . *
func parseNamespace(ns string) string {
	reg, _ := regexp.Compile("[^a-zA-Z0-9-]+")
	clean := reg.ReplaceAllString(ns, "")
	if len(clean) == 0 {
		return "default"
	}
	if len(clean) > MaxNamespaceLength {
		return clean[0 : MaxNamespaceLength-1]
	}
	return clean
}

// private and not guarded by mutex locks, since called by other guarded methods

func qualifiedId(join joinPayload) string {
	return join.origin + "#" + join.RoomId
}

func newRoom(qualifiedId string, join joinPayload) *trialRoom {
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
	namespace := parseNamespace(join.Namespace)
	helpers.EnsureDir("./data/" + namespace)

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
		namespace:           namespace,
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

func (r *trialRoom) countdown() {
	// blocking "end" event and delete
	endTimer := time.NewTimer(time.Duration(r.duration) * time.Second)
	<-endTimer.C

	// listened by peer_conn
	close(r.endCh)

	for _, ps := range r.peerServerIndex {
		go ps.ws.SendWithPayload("end", r.files())
	}
	log.Printf("[room %s] end\n", r.shortId)

	r.delete()
}

// API read-write

func joinRoom(join joinPayload) (*trialRoom, error) {
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
			log.Printf("[room %s] joined\n", qualifiedId)
			return r, nil
		}
	} else {
		log.Printf("[room %s] created\n", qualifiedId)
		newRoom := newRoom(qualifiedId, join)
		roomIndex[qualifiedId] = newRoom
		return newRoom, nil
	}
}

func (r *trialRoom) incInTracksReadyCount() {
	r.Lock()
	defer r.Unlock()

	if r.inTracksReadyCount == r.neededTracks {
		// reconnection case
		return
	}

	r.inTracksReadyCount++
	log.Printf("[room %s] track updated count: %d\n", r.shortId, r.inTracksReadyCount)

	if r.inTracksReadyCount == r.neededTracks {
		log.Printf("[room %s] users are ready\n", r.shortId)
		close(r.waitForAllCh)
		r.started = true
		r.startedAt = time.Now()
		for _, ps := range r.peerServerIndex {
			go ps.ws.Send("start")
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
		go r.mixer.managedUpdateSignaling()
	}
}

func (r *trialRoom) bindPeerServer(ps *peerServer) {
	r.Lock()
	defer func() {
		r.Unlock()
		r.mixer.managedUpdateSignaling()
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
		if r.userCount() == 1 && !r.started {
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
	userId string,
	join joinPayload,
	pc *peerConn,
	remoteTrack *webrtc.TrackRemote,
	receiver *webrtc.RTPReceiver,
) {
	outputTrack, err := r.mixer.newLocalTrackFromRemote(userId, r, join, pc, remoteTrack, receiver)

	if err != nil {
		log.Printf("[room %s error] runLocalTrackFromRemote: %v\n", r.shortId, err)
	} else {
		defer r.mixer.removeLocalTrack(outputTrack.id)

		// needed to relay control fx events between peer server and output track
		ps := r.peerServerIndex[userId]
		ps.setLocalTrack(remoteTrack.Kind().String(), outputTrack)

		// will trigger signaling if needed
		r.incOutTracksReadyCount()

		outputTrack.loop() // blocking
	}
}
