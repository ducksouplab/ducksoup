package sfu

import (
	"errors"
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const (
	DefaultSize          = 2
	MaxSize              = 8
	DefaultTracksPerPeer = 2
	DefaultDuration      = 30
	MaxDuration          = 1200
	Ending               = 10
	MaxNamespaceLength   = 30
)

// global state
var (
	mu        sync.Mutex // TODO init here
	roomIndex map[string]*Room
)

// room holds all the resources of a given experiment, accepting an exact number of *size* attendees
type Room struct {
	sync.RWMutex
	// guarded by mutex
	mixer            *Mixer
	peerServerIndex  map[string]*PeerServer // per user id
	connectedIndex   map[string]bool        // per user id, undefined: never connected, false: previously connected, true: connected
	joinedCountIndex map[string]int         // per user id
	filesIndex       map[string][]string    // per user id, contains media file names
	started          bool
	startedAt        time.Time
	tracksReadyCount int
	// channels (safe)
	waitForAllCh chan struct{}
	endCh        chan struct{}
	// other (written only during initialization)
	qualifiedId   string
	shortId       string
	namespace     string
	size          int
	tracksPerPeer int
	duration      int
}

func init() {
	mu = sync.Mutex{}
	roomIndex = make(map[string]*Room)
}

func (r *Room) delete() {
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

func QualifiedId(joinPayload JoinPayload) string {
	return joinPayload.origin + "#" + joinPayload.RoomId
}

func newRoom(qualifiedId string, joinPayload JoinPayload) *Room {
	// process duration
	duration := joinPayload.Duration
	if duration < 1 {
		duration = DefaultDuration
	} else if duration > MaxDuration {
		duration = MaxDuration
	}

	// process size
	size := joinPayload.Size
	if size < 1 {
		size = DefaultSize
	} else if size > MaxSize {
		size = MaxSize
	}

	// room initialized with one connected peer
	connectedIndex := make(map[string]bool)
	connectedIndex[joinPayload.UserId] = true
	joinedCountIndex := make(map[string]int)
	joinedCountIndex[joinPayload.UserId] = 1

	// create folder for logs
	namespace := parseNamespace(joinPayload.Namespace)
	helpers.EnsureDir("./logs/" + namespace)

	shortId := joinPayload.RoomId

	return &Room{
		peerServerIndex:  make(map[string]*PeerServer),
		filesIndex:       make(map[string][]string),
		connectedIndex:   connectedIndex,
		joinedCountIndex: joinedCountIndex,
		waitForAllCh:     make(chan struct{}),
		endCh:            make(chan struct{}),
		tracksReadyCount: 0,
		mixer:            newMixer(shortId),
		qualifiedId:      qualifiedId,
		shortId:          shortId,
		namespace:        namespace,
		size:             size,
		tracksPerPeer:    DefaultTracksPerPeer,
		duration:         duration,
	}
}

func (r *Room) userCount() int {
	return len(r.connectedIndex)
}

func (r *Room) countdown() {
	// blocking "end" event and delete
	endTimer := time.NewTimer(time.Duration(r.duration) * time.Second)
	<-endTimer.C

	// listened by peer_conn
	close(r.endCh)

	for _, ps := range r.peerServerIndex {
		go ps.wsConn.SendWithPayload("end", r.Files())
	}
	log.Printf("[room %s] end\n", r.shortId)

	r.delete()
}

// API read-write

func JoinRoom(joinPayload JoinPayload) (*Room, error) {
	// guard `roomIndex`
	mu.Lock()
	defer mu.Unlock()

	qualifiedId := QualifiedId(joinPayload)
	userId := joinPayload.UserId

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
		newRoom := newRoom(qualifiedId, joinPayload)
		roomIndex[qualifiedId] = newRoom
		return newRoom, nil
	}
}

func (r *Room) IncTracksReadyCount() {
	r.Lock()
	defer r.Unlock()

	neededTracks := r.size * r.tracksPerPeer

	if r.tracksReadyCount == neededTracks {
		// reconnection case
		return
	}

	r.tracksReadyCount++
	log.Printf("[room %s] track updated count: %d\n", r.shortId, r.tracksReadyCount)

	if r.tracksReadyCount == neededTracks {
		log.Printf("[room %s] users are ready\n", r.shortId)
		close(r.waitForAllCh)
		r.started = true
		r.startedAt = time.Now()
		for _, ps := range r.peerServerIndex {
			go ps.wsConn.Send("start")
		}
		go r.countdown()
		return
	}
}

func (r *Room) Bind(ps *PeerServer) {
	r.Lock()
	defer r.Unlock()

	r.peerServerIndex[ps.userId] = ps
}

func (r *Room) Unbind(ps *PeerServer) {
	r.Lock()
	defer r.Unlock()

	delete(r.peerServerIndex, ps.userId)
}

func (r *Room) DisconnectUser(userId string) {
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

func (r *Room) AddFiles(userId string, files []string) {
	r.Lock()
	defer r.Unlock()

	r.filesIndex[userId] = append(r.filesIndex[userId], files...)
}

// API read

func (r *Room) JoinedCountForUser(userId string) int {
	r.RLock()
	defer r.RUnlock()

	return r.joinedCountIndex[userId]
}

func (r *Room) Files() map[string][]string {
	r.RLock()
	defer r.RUnlock()

	return r.filesIndex
}

func (r *Room) EndingDelay() (delay int) {
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

func (r *Room) AddTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	r.Lock()
	defer func() {
		r.Unlock()
		r.UpdateSignaling()
	}()
	return r.mixer.addTrack(t)
}

func (r *Room) RemoveTrack(t *webrtc.TrackLocalStaticRTP) {
	r.Lock()
	defer func() {
		r.Unlock()
		r.UpdateSignaling()
	}()
	r.mixer.removeTrack(t)
}

// Update each PeerConnection so that it is getting all the expected media tracks
func (r *Room) UpdateSignaling() {
	r.Lock()
	defer func() {
		r.Unlock()
		go r.dispatchKeyFrame()
	}()

	log.Printf("[room %s] signaling update\n", r.shortId)

signalingLoop:
	for tries := 0; ; tries++ {
		switch r.mixer.updateSignalingState(r) {
		case SignalingOk:
			break signalingLoop
		case SignalingRetryWithDelay:
			time.Sleep(time.Second * 1)
		}
		// case SignalingRetryNow -> continue loop

		if tries == 25 {
			// release the lock and attempt a sync in 3 seconds. We might be blocking a RemoveTrack or AddTrack
			go func() {
				time.Sleep(time.Second * 3)
				r.UpdateSignaling()
			}()
			return
		}
	}
}

// dispatchKeyFrame sends a keyframe to all PeerConnections, used everytime a new user joins the call
func (r *Room) dispatchKeyFrame() {
	r.RLock()
	defer r.RUnlock()

	log.Printf("[room %s] dispatchKeyFrame\n", r.shortId)

	for _, ps := range r.peerServerIndex {
		for _, receiver := range ps.peerConn.GetReceivers() {
			if receiver.Track() == nil {
				continue
			}

			_ = ps.peerConn.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: uint32(receiver.Track().SSRC()),
				},
			})
		}
	}
}
