package sfu

import (
	"encoding/json"
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
	Finishing            = 10
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
	peerServerIndex  map[string]*PeerServer                 // per user id
	connectedIndex   map[string]bool                        // per user id, undefined: never connected, false: previously connected, true: connected
	joinedCountIndex map[string]int                         // per user id
	filesIndex       map[string][]string                    // per user id, contains media file names
	trackIndex       map[string]*webrtc.TrackLocalStaticRTP // per track id
	startedAt        time.Time
	tracksReadyCount int
	// channels (safe)
	waitForAllCh chan struct{}
	finishCh     chan struct{}
	// other (written only during initialization)
	id            string
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

	log.Printf("[room %s] deleted\n", r.id)
	delete(roomIndex, r.id)
}

// remove special characters like / . *
func parseNamespace(ns string) string {
	reg, _ := regexp.Compile("[^a-zA-Z0-9]+")
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

func newRoom(joinPayload JoinPayload) *Room {
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

	return &Room{
		peerServerIndex:  make(map[string]*PeerServer),
		filesIndex:       make(map[string][]string),
		connectedIndex:   connectedIndex,
		joinedCountIndex: joinedCountIndex,
		trackIndex:       map[string]*webrtc.TrackLocalStaticRTP{},
		waitForAllCh:     make(chan struct{}),
		finishCh:         make(chan struct{}),
		tracksReadyCount: 0,
		id:               joinPayload.Room,
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
	// blocking "finish" event and delete
	finishTimer := time.NewTimer(time.Duration(r.duration) * time.Second)
	<-finishTimer.C
	log.Printf("[room %s] finish\n", r.id)
	close(r.finishCh)
	r.delete()
}

// API read-write

func JoinRoom(joinPayload JoinPayload) (*Room, error) {
	// guard `roomIndex`
	mu.Lock()
	defer mu.Unlock()

	roomId := joinPayload.Room
	userId := joinPayload.UserId

	if r, ok := roomIndex[roomId]; ok {
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
			log.Printf("[room %s] joined\n", roomId)
			return r, nil
		}
	} else {
		log.Printf("[room %s] created\n", roomId)
		newRoom := newRoom(joinPayload)
		roomIndex[roomId] = newRoom
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
	log.Printf("[room %s] new track, updated count: %d\n", r.id, r.tracksReadyCount)

	if r.tracksReadyCount == neededTracks {
		log.Printf("[room %s] closing waitForAllCh\n", r.id)
		close(r.waitForAllCh)
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

func (r *Room) DisconnectUser(userId string) {
	r.Lock()
	defer r.Unlock()

	// protects decrementing since RemovePeer maybe called several times for same user
	if r.connectedIndex[userId] {
		// remove user current connection details (=peerServer)
		delete(r.peerServerIndex, userId)
		// mark disconnected, but keep track of her
		r.connectedIndex[userId] = false
		if r.userCount() == 1 {
			// don't keep this room
			r.delete()
		}
	}
}

// Add to list of tracks and fire renegotation for all PeerConnections
func (r *Room) AddProcessedTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	r.Lock()
	defer func() {
		r.Unlock()
		r.UpdateSignaling()
	}()

	// Create a new TrackLocal with the same codec as the incoming one
	track, err := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), t.StreamID())
	if err != nil {
		panic(err)
	}

	r.trackIndex[t.ID()] = track
	return track
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (r *Room) RemoveProcessedTrack(t *webrtc.TrackLocalStaticRTP) {
	r.Lock()
	defer func() {
		r.Unlock()
		r.UpdateSignaling()
	}()

	delete(r.trackIndex, t.ID())
}

func (r *Room) AddFiles(userId string, files []string) {
	r.Lock()
	defer r.Unlock()

	r.filesIndex[userId] = append(r.filesIndex[userId], files...)
}

// Update each PeerConnection so that it is getting all the expected media tracks
func (r *Room) UpdateSignaling() {
	r.Lock()
	defer func() {
		r.Unlock()
		r.DispatchKeyFrame()
	}()

	log.Printf("[room %s] signaling update\n", r.id)
	tryUpdateSignaling := func() (success bool) {
		for userId, ps := range r.peerServerIndex {

			peerConn := ps.peerConn

			if peerConn.ConnectionState() == webrtc.PeerConnectionStateClosed {
				delete(r.peerServerIndex, userId)
				break
			}

			// map of sender we are already sending, so we don't double send
			existingSenders := map[string]bool{}

			for _, sender := range peerConn.GetSenders() {
				if sender.Track() == nil {
					continue
				}

				existingSenders[sender.Track().ID()] = true

				// if we have a RTPSender that doesn't map to an existing track remove and signal
				_, ok := r.trackIndex[sender.Track().ID()]
				if !ok {
					if err := peerConn.RemoveTrack(sender); err != nil {
						return false
					}
				}
			}

			// when room size is 1, it acts as a mirror
			if r.size != 1 {
				// don't receive videos we are sending, make sure we don't have loopback (remote peer point of view)
				for _, receiver := range peerConn.GetReceivers() {
					if receiver.Track() == nil {
						continue
					}
					existingSenders[receiver.Track().ID()] = true
				}
			}

			// add all track we aren't sending yet to the PeerConnection
			for trackID := range r.trackIndex {
				if _, ok := existingSenders[trackID]; !ok {
					rtpSender, err := peerConn.AddTrack(r.trackIndex[trackID])

					if err != nil {
						return false
					}

					// TODO check if needed
					// Read incoming RTCP packets
					// Before these packets are returned they are processed by interceptors. For things
					// like NACK this needs to be called.
					go func() {
						rtcpBuf := make([]byte, 1500)
						for {
							if _, _, rtcpErr := rtpSender.Read(rtcpBuf); rtcpErr != nil {
								return
							}
						}
					}()
				}
			}

			offer, err := peerConn.CreateOffer(nil)
			if err != nil {
				log.Printf("[room %s] CreateOffer failed: %v\n", r.id, err)
				return false
			}

			if err = peerConn.SetLocalDescription(offer); err != nil {
				return false
			}

			offerString, err := json.Marshal(offer)
			if err != nil {
				return false
			}

			if err = ps.wsConn.SendWithPayload("offer", string(offerString)); err != nil {
				return false
			}
		}

		return true
	}

	for tries := 0; ; tries++ {
		if tries == 25 {
			// release the lock and attempt a sync in 3 seconds. We might be blocking a RemoveTrack or AddTrack
			go func() {
				time.Sleep(time.Second * 3)
				r.UpdateSignaling()
			}()
			return
		}
		// don't try again if succeeded
		if tryUpdateSignaling() {
			break
		}
	}
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

func (r *Room) FinishingDelay() (delay int) {
	r.RLock()
	defer r.RUnlock()

	elapsed := time.Since(r.startedAt)

	remaining := r.duration - int(elapsed.Seconds())
	delay = remaining - Finishing
	if delay < 1 {
		delay = 1
	}
	return
}

// dispatchKeyFrame sends a keyframe to all PeerConnections, used everytime a new user joins the call
func (r *Room) DispatchKeyFrame() {
	r.RLock()
	defer r.RUnlock()

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
