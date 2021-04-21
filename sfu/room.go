// sfu global state (tracks & peers) providing functions for files in same package
package sfu

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

// global state
var (
	mu    sync.Mutex
	rooms map[string]*Room
)

type Peer struct {
	rtcConn *webrtc.PeerConnection
	wsConn  *WebsocketConn
}

type Room struct {
	sync.Mutex
	id              string
	peers           []Peer
	processedTracks map[string]*webrtc.TrackLocalStaticRTP
}

func init() {
	mu = sync.Mutex{}
	rooms = make(map[string]*Room)
}

func GetRoom(id string) *Room {
	mu.Lock()
	defer mu.Unlock()

	if r, ok := rooms[id]; ok {
		fmt.Println("Exists room")
		return r
	} else {
		fmt.Println("Creates room")
		newRoom := NewRoom(id)
		rooms[id] = newRoom
		return newRoom
	}
}

func NewRoom(id string) *Room {
	room := &Room{
		id:              id,
		processedTracks: map[string]*webrtc.TrackLocalStaticRTP{},
	}

	// request a keyframe every 3 seconds
	go func() {
		for range time.NewTicker(time.Second * 3).C {
			room.DispatchKeyFrame()
		}
	}()

	return room
}

func (r *Room) AddPeer(rtcConn *webrtc.PeerConnection, wsConn *WebsocketConn) {
	r.Lock()
	defer r.Unlock()

	r.peers = append(r.peers, Peer{rtcConn, wsConn})
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
		for i := range r.peers {
			pc := r.peers[i].rtcConn
			if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
				r.peers = append(r.peers[:i], r.peers[i+1:]...)
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

			if err = r.peers[i].wsConn.WriteJSON(&Message{
				Event: "offer",
				Data:  string(offerString),
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

	for i := range r.peers {
		for _, receiver := range r.peers[i].rtcConn.GetReceivers() {
			if receiver.Track() == nil {
				continue
			}

			_ = r.peers[i].rtcConn.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: uint32(receiver.Track().SSRC()),
				},
			})
		}
	}
}
