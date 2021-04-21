// sfu global state (tracks & peers) providing functions for files in same package
package sfu

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

var (
	// lock for peerConnections and trackLocals
	tracksLock      sync.RWMutex
	peerConnections []peerConnectionState
	videoTracks     map[string]*webrtc.TrackLocalStaticRTP
	audioTracks     map[string]*webrtc.TrackLocalStaticRTP
)

type peerConnectionState struct {
	peerConnection *webrtc.PeerConnection
	websocket      *Conn
}

type websocketMessage struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

func init() {
	videoTracks = map[string]*webrtc.TrackLocalStaticRTP{}
	audioTracks = map[string]*webrtc.TrackLocalStaticRTP{}

	// request a keyframe every 3 seconds
	go func() {
		for range time.NewTicker(time.Second * 3).C {
			dispatchKeyFrame()
		}
	}()
}

// Add to list of tracks and fire renegotation for all PeerConnections
func addVideoTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		signalingUpdate()
	}()

	// Create a new TrackLocal with the same codec as our incoming
	track, err := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), t.StreamID())
	if err != nil {
		panic(err)
	}

	videoTracks[t.ID()] = track
	return track
}

// Remove from list of tracks and fire renegotation for all PeerConnections
func removeVideoTrack(t *webrtc.TrackLocalStaticRTP) {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		signalingUpdate()
	}()

	delete(videoTracks, t.ID())
}

func addAudioTrack(t *webrtc.TrackRemote) *webrtc.TrackLocalStaticRTP {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		signalingUpdate()
	}()

	// Create a new TrackLocal with the same codec as our incoming
	track, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{
		MimeType:     "audio/opus",
		ClockRate:    48000,
		Channels:     0,
		SDPFmtpLine:  "stereo=0;minptime=10;useinbandfec=1",
		RTCPFeedback: nil,
	}, t.ID(), t.StreamID())

	if err != nil {
		panic(err)
	}

	audioTracks[t.ID()] = track
	return track
}

func removeAudioTrack(t *webrtc.TrackLocalStaticRTP) {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		signalingUpdate()
	}()

	delete(audioTracks, t.ID())
}

// Update each PeerConnection so that it is getting all the expected media tracks
func signalingUpdate() {
	tracksLock.Lock()
	defer func() {
		tracksLock.Unlock()
		dispatchKeyFrame()
	}()

	attemptSync := func() (tryAgain bool) {
		for i := range peerConnections {
			pc := peerConnections[i].peerConnection
			if pc.ConnectionState() == webrtc.PeerConnectionStateClosed {
				peerConnections = append(peerConnections[:i], peerConnections[i+1:]...)
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
				_, videoOk := videoTracks[sender.Track().ID()]
				_, audioOk := audioTracks[sender.Track().ID()]
				if !videoOk && !audioOk {
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
			for trackID := range videoTracks {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := pc.AddTrack(videoTracks[trackID]); err != nil {
						return true
					}
				}
			}
			for trackID := range audioTracks {
				if _, ok := existingSenders[trackID]; !ok {
					if _, err := pc.AddTrack(audioTracks[trackID]); err != nil {
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

			if err = peerConnections[i].websocket.WriteJSON(&websocketMessage{
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
				signalingUpdate()
			}()
			return
		}

		if !attemptSync() {
			break
		}
	}
}

// dispatchKeyFrame sends a keyframe to all PeerConnections, used everytime a new user joins the call
func dispatchKeyFrame() {
	tracksLock.Lock()
	defer tracksLock.Unlock()

	for i := range peerConnections {
		for _, receiver := range peerConnections[i].peerConnection.GetReceivers() {
			if receiver.Track() == nil {
				continue
			}

			_ = peerConnections[i].peerConnection.WriteRTCP([]rtcp.Packet{
				&rtcp.PictureLossIndication{
					MediaSSRC: uint32(receiver.Track().SSRC()),
				},
			})
		}
	}
}
