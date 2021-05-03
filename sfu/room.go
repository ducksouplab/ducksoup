package sfu

import (
	"encoding/json"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const (
	DefaultSize          = 2
	DefaultTracksPerPeer = 2
	DefaultDuration      = 30
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
	peerServers     []*PeerServer
	processedTracks map[string]*webrtc.TrackLocalStaticRTP
	// atomic operations only
	tracksReadyCount uint32
	peerCount        uint32
	// channels
	waitForAllCh chan struct{}
	stopCh       chan struct{}
	// other
	id            string
	size          uint32
	tracksPerPeer uint32
	duration      uint32
}

func init() {
	mu = sync.Mutex{}
	rooms = make(map[string]*Room)
}

func (r *Room) IncTracksReadyCount() {
	atomic.AddUint32(&r.tracksReadyCount, 1)
	log.Printf("[room] track ready update %d\n", r.tracksReadyCount)

	if r.tracksReadyCount == r.size*r.tracksPerPeer {
		close(r.waitForAllCh)
		go r.planStop()
		return
	}
}

func (r *Room) planStop() {
	timer := time.NewTimer(time.Duration(r.duration) * time.Second)
	<-timer.C
	close(r.stopCh)
	r.Delete()
}

func newRoom(id string) *Room {
	room := &Room{
		waitForAllCh:     make(chan struct{}),
		stopCh:           make(chan struct{}),
		id:               id,
		processedTracks:  map[string]*webrtc.TrackLocalStaticRTP{},
		size:             DefaultSize,
		peerCount:        1,
		tracksPerPeer:    DefaultTracksPerPeer,
		tracksReadyCount: 0,
		duration:         DefaultDuration,
	}

	return room
}

func JoinRoom(id string) (*Room, error) {
	// guard `rooms`
	mu.Lock()
	defer mu.Unlock()

	if r, ok := rooms[id]; ok {
		if r.peerCount == r.size {
			return nil, errors.New("limit reached")
		} else {
			atomic.AddUint32(&r.peerCount, 1)
			log.Printf("[ws] joined existing room: %s\n", id)
			return r, nil
		}
	} else {
		log.Printf("[ws] joined new room: %s\n", id)
		newRoom := newRoom(id)
		rooms[id] = newRoom
		return newRoom, nil
	}
}

func (r *Room) Delete() {
	// guard `rooms`
	mu.Lock()
	defer mu.Unlock()

	delete(rooms, r.id)
}

func (r *Room) AddPeerServer(ps *PeerServer) {
	r.Lock()
	defer r.Unlock()

	r.peerServers = append(r.peerServers, ps)
}

func (r *Room) PeerQuit() {
	if r.peerCount == 1 {
		r.Delete()
	}
	// else let the room run and let the possibility to recover if same user joins again (TODO)
}

// Add to list of tracks and fire renegotation for all PeerConnections
func (r *Room) AddProcessedTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	r.Lock()
	defer func() {
		r.Unlock()
		r.SignalingUpdate()
	}()

	// Create a new TrackLocal with the same codec as the incoming one
	track, err := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), t.StreamID())
	if err != nil {
		panic(err)
	}

	r.processedTracks[t.ID()] = track
	return track
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func (r *Room) RemoveProcessedTrack(t *webrtc.TrackLocalStaticRTP) {
	r.Lock()
	defer func() {
		r.Unlock()
		r.SignalingUpdate()
	}()

	delete(r.processedTracks, t.ID())
}

// Update each PeerConnection so that it is getting all the expected media tracks
func (r *Room) SignalingUpdate() {
	log.Println("[room] signaling update")
	r.Lock()
	defer func() {
		r.Unlock()
		r.DispatchKeyFrame()
	}()

	attemptSync := func() (tryAgain bool) {
		for i := range r.peerServers {
			peerServer := r.peerServers[i]
			peerConn := peerServer.peerConn
			if peerConn.ConnectionState() == webrtc.PeerConnectionStateClosed {
				r.peerServers = append(r.peerServers[:i], r.peerServers[i+1:]...)
				return true // We modified the slice, start from the beginning
			}

			// map of sender we already are sending, so we don't double send
			existingSenders := map[string]bool{}

			for _, sender := range peerConn.GetSenders() {
				if sender.Track() == nil {
					continue
				}

				existingSenders[sender.Track().ID()] = true

				// If we have a RTPSender that doesn't map to a existing track remove and signal
				_, ok := r.processedTracks[sender.Track().ID()]
				if !ok {
					if err := peerConn.RemoveTrack(sender); err != nil {
						return true
					}
				}
			}

			// Don't receive videos we are sending, make sure we don't have loopback (remote peer point of view)
			for _, receiver := range peerConn.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}
				existingSenders[receiver.Track().ID()] = true
			}

			// Add all track we aren't sending yet to the PeerConnection
			for trackID := range r.processedTracks {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := peerConn.AddTrack(r.processedTracks[trackID]); err != nil {
						return true
					}
				}
			}

			offer, err := peerConn.CreateOffer(nil)
			if err != nil {
				return true
			}
			if err = peerConn.SetLocalDescription(offer); err != nil {
				return true
			}

			offerString, err := json.Marshal(offer)
			if err != nil {
				return true
			}

			if err = r.peerServers[i].wsConn.WriteJSON(&Message{
				Type:    "offer",
				Payload: string(offerString),
			}); err != nil {
				return true
			}
		}

		return
	}

	for syncAttempt := 0; ; syncAttempt++ {
		if syncAttempt == 25 {
			// Release the lock and attempt a sync in 3 seconds. We might be blocking a RemoveTrack or AddTrack
			go func() {
				time.Sleep(time.Second * 3)
				r.SignalingUpdate()
			}()
			return
		}

		if !attemptSync() {
			break
		}
	}
}

// dispatchKeyFrame sends a keyframe to all PeerConnections, used everytime a new user joins the call
func (r *Room) DispatchKeyFrame() {
	r.Lock()
	defer r.Unlock()

	for i := range r.peerServers {
		for _, receiver := range r.peerServers[i].peerConn.GetReceivers() {
			if receiver.Track() == nil {
				continue
			}

			_ = r.peerServers[i].peerConn.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: uint32(receiver.Track().SSRC()),
				},
			})
		}
	}
}
