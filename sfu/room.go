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
)

// global state
var (
	mu    sync.Mutex // TODO init here
	rooms map[string]*Room
)

// Room holds all the resources of a given experiment, accepting an exact number of *size* attendees
type Room struct {
	// embedded
	sync.Mutex
	// channels
	readyCh  chan struct{}
	holdOnCh chan struct{}
	stopCh   chan struct{}
	// state
	id               string
	peerServers      []*PeerServer
	processedTracks  map[string]*webrtc.TrackLocalStaticRTP
	size             uint
	peerCount        uint
	tracksPerPeer    uint
	tracksReadyCount uint
	duration         uint
}

func init() {
	mu = sync.Mutex{}
	rooms = make(map[string]*Room)
}

func (r *Room) incTracksReadyCount() {
	mu.Lock()
	defer mu.Unlock()

	r.tracksReadyCount += 1
}

func (r *Room) readyLoop() {
	for {
		<-r.readyCh
		r.incTracksReadyCount()

		log.Printf("[room] ready update %d\n", r.tracksReadyCount)

		if r.tracksReadyCount == r.size*r.tracksPerPeer {
			close(r.holdOnCh)
			go r.planStop()
			return
		}
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
		readyCh:          make(chan struct{}),
		holdOnCh:         make(chan struct{}),
		stopCh:           make(chan struct{}),
		id:               id,
		processedTracks:  map[string]*webrtc.TrackLocalStaticRTP{},
		size:             DefaultSize,
		peerCount:        1,
		tracksPerPeer:    DefaultTracksPerPeer,
		tracksReadyCount: 0,
		duration:         DefaultDuration,
	}

	go room.readyLoop()

	return room
}

func JoinRoom(id string) (*Room, error) {
	mu.Lock()
	defer mu.Unlock()

	if r, ok := rooms[id]; ok {
		if r.peerCount == r.size {
			return nil, errors.New("limit reached")
		} else {
			r.Lock()
			defer r.Unlock()
			r.peerCount += 1
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
	mu.Lock()
	defer mu.Unlock()

	delete(rooms, r.id)
}

func (r *Room) AddPeerServer(ps *PeerServer) {
	r.Lock()
	defer r.Unlock()

	r.peerServers = append(r.peerServers, ps)
}

// Add to list of tracks and fire renegotation for all PeerConnections
func (r *Room) AddProcessedTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	r.Lock()
	defer func() {
		r.Unlock()
		r.SignalingUpdate()
	}()

	// Create a new TrackLocal with the same codec as our incoming
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
	r.Lock()
	defer func() {
		r.Unlock()
		r.DispatchKeyFrame()
	}()

	attemptSync := func() (tryAgain bool) {
		for i := range r.peerServers {
			pc := r.peerServers[i].peerConn
			if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
				r.peerServers = append(r.peerServers[:i], r.peerServers[i+1:]...)
				return true // We modified the slice, start from the beginning
			}

			// map of sender we already are sending, so we don't double send
			existingSenders := map[string]bool{}

			for _, sender := range pc.GetSenders() {
				if sender.Track() == nil {
					continue
				}

				existingSenders[sender.Track().ID()] = true

				// If we have a RTPSender that doesn't map to a existing track remove and signal
				_, ok := r.processedTracks[sender.Track().ID()]
				if !ok {
					if err := pc.RemoveTrack(sender); err != nil {
						return true
					}
				}
			}

			// Don't receive videos we are sending, make sure we don't have loopback (remote peer point of view)
			for _, receiver := range pc.GetReceivers() {
				if receiver.Track() == nil {
					continue
				}
				existingSenders[receiver.Track().ID()] = true
			}

			// Add all track we aren't sending yet to the PeerConnection
			for trackID := range r.processedTracks {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := pc.AddTrack(r.processedTracks[trackID]); err != nil {
						return true
					}
				}
			}

			offer, err := pc.CreateOffer(nil)
			if err != nil {
				return true
			}

			if err = pc.SetLocalDescription(offer); err != nil {
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
