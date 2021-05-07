package sfu

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const (
	DefaultSize          = 2
	DefaultTracksPerPeer = 2
	DefaultDuration      = 30
	MaxDuration          = 1200
)

// global state
var (
	mu    sync.Mutex // TODO init here
	rooms map[string]*Room
)

// room holds all the resources of a given experiment, accepting an exact number of *size* attendees
type Room struct {
	sync.Mutex
	// guarded by mutex
	peerServerIndex  map[string]*PeerServer
	connectedIndex   map[string]bool // undefined: never connected, false: previously connected, true: connected
	joinedCountIndex map[string]uint32
	trackIndex       map[string]*webrtc.TrackLocalStaticRTP
	// atomic operations only
	tracksReadyCount uint32
	peerCount        uint32
	// channels (safe)
	waitForAllCh chan struct{}
	stopCh       chan struct{}
	// other (written only when initializing)
	id            string
	size          uint32
	tracksPerPeer uint32
	duration      uint32
}

func init() {
	mu = sync.Mutex{}
	rooms = make(map[string]*Room)
}

func newRoom(joinPayload JoinPayload) *Room {
	// process duration
	duration := joinPayload.Duration
	if duration < 1 {
		duration = DefaultDuration
	} else if duration > MaxDuration {
		duration = MaxDuration
	}

	// room initialized with one connected peer
	connectedIndex := make(map[string]bool)
	connectedIndex[joinPayload.UserId] = true
	joinedCountIndex := make(map[string]uint32)
	joinedCountIndex[joinPayload.UserId] = 1

	return &Room{
		peerServerIndex:  make(map[string]*PeerServer),
		connectedIndex:   connectedIndex,
		joinedCountIndex: joinedCountIndex,
		trackIndex:       map[string]*webrtc.TrackLocalStaticRTP{},
		waitForAllCh:     make(chan struct{}),
		stopCh:           make(chan struct{}),
		id:               joinPayload.Room,
		size:             DefaultSize,
		peerCount:        1,
		tracksPerPeer:    DefaultTracksPerPeer,
		tracksReadyCount: 0,
		duration:         duration,
	}
}

func JoinRoom(joinPayload JoinPayload) (*Room, error) {
	// guard `rooms`
	mu.Lock()
	defer mu.Unlock()

	roomId := joinPayload.Room
	userId := joinPayload.UserId

	if r, ok := rooms[roomId]; ok {
		r.Lock()
		defer r.Unlock()
		if connected, ok := r.connectedIndex[userId]; ok {
			// same user has previously connected
			if connected {
				// forbidden (for instance: second browser tab)
				return nil, errors.New("already connected")
			} else {
				// reconnects (for instance: page reload)
				r.connectedIndex[userId] = true
				r.joinedCountIndex[userId]++
				r.peerCount++
				return r, nil
			}
		} else if r.peerCount == r.size {
			// room limit reached
			return nil, errors.New("limit reached")
		} else {
			// new user joined existing room
			r.connectedIndex[userId] = true
			r.joinedCountIndex[userId] = 1
			r.peerCount++
			log.Printf("[room #%s] joined\n", roomId)
			return r, nil
		}
	} else {
		log.Printf("[room #%s] created\n", roomId)
		newRoom := newRoom(joinPayload)
		rooms[roomId] = newRoom
		return newRoom, nil
	}
}

func (r *Room) Delete() {
	// guard `rooms`
	mu.Lock()
	defer mu.Unlock()

	log.Printf("[room #%s] deleted\n", r.id)
	delete(rooms, r.id)
}

func (r *Room) IncTracksReadyCount() {
	r.Lock()
	defer r.Unlock()

	log.Printf("[room #%s] new track, current count: %d\n", r.id, r.tracksReadyCount)

	neededTracks := r.size * r.tracksPerPeer

	if r.tracksReadyCount == neededTracks {
		// reconnection case
		return
	}

	r.tracksReadyCount++
	log.Printf("[room #%s] new track, updated count: %d\n", r.id, r.tracksReadyCount)

	if r.tracksReadyCount == neededTracks {
		log.Printf("[room #%s] closing waitForAllCh\n", r.id)
		close(r.waitForAllCh)
		for _, ps := range r.peerServerIndex {
			go ps.wsConn.Send("start")
		}
		go r.timeLimit()
		return
	}
}

func (r *Room) timeLimit() {
	timer := time.NewTimer(time.Duration(r.duration) * time.Second)
	<-timer.C
	log.Printf("[room #%s] closing stopCh\n", r.id)
	close(r.stopCh)
	r.Delete()
}

func (r *Room) AddPeer(ps *PeerServer) {
	r.Lock()
	defer r.Unlock()

	r.peerServerIndex[ps.userId] = ps
}

func (r *Room) RemovePeer(userId string) {
	r.Lock()
	defer r.Unlock()

	// protects decrementing since RemovePeer maybe called several times for same user
	if r.connectedIndex[userId] {
		if r.peerCount == 1 {
			r.Delete()
		} else {
			delete(r.peerServerIndex, userId)
			r.connectedIndex[userId] = false
			r.peerCount--
		}
	}
}

func (r *Room) UserJoinedCount(userId string) uint32 {
	r.Lock()
	defer r.Unlock()

	return r.joinedCountIndex[userId]
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

// Update each PeerConnection so that it is getting all the expected media tracks
func (r *Room) UpdateSignaling() {
	r.Lock()
	defer func() {
		r.Unlock()
		r.DispatchKeyFrame()
	}()

	log.Printf("[room #%s] signaling update\n", r.id)
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

			// don't receive videos we are sending, make sure we don't have loopback (remote peer point of view)
			for _, receiver := range peerConn.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}
				existingSenders[receiver.Track().ID()] = true
			}

			// add all track we aren't sending yet to the PeerConnection
			for trackID := range r.trackIndex {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := peerConn.AddTrack(r.trackIndex[trackID]); err != nil {
						return false
					}
				}
			}

			offer, err := peerConn.CreateOffer(nil)
			if err != nil {
				return false
			}

			if err = peerConn.SetLocalDescription(offer); err != nil {
				return false
			}

			offerString, err := json.Marshal(offer)
			if err != nil {
				return false
			}

			if err = ps.wsConn.SendJSON(&Message{
				Type:    "offer",
				Payload: string(offerString),
			}); err != nil {
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

// dispatchKeyFrame sends a keyframe to all PeerConnections, used everytime a new user joins the call
func (r *Room) DispatchKeyFrame() {
	r.Lock()
	defer r.Unlock()

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
