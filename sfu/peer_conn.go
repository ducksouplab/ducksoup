package sfu

import (
	"encoding/json"
	"log"

	"github.com/creamlab/ducksoup/engine"
	"github.com/pion/webrtc/v3"
)

// Augmented pion PeerConnection
type peerConn struct {
	*webrtc.PeerConnection
	// if peer connection is closed before room is ended (for instance on browser page refresh)
	closedCh chan struct{}
}

// API

func newPionPeerConn(userId string, videoCodec string) (ppc *webrtc.PeerConnection, err error) {
	// create RTC API with chosen codecs
	api, err := engine.NewWebRTCAPI()
	if err != nil {
		log.Printf("[user %s] NewWebRTCAPI codecs: %v\n", userId, err)
		return
	}
	// configure and create a new RTCPeerConnection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	ppc, err = api.NewPeerConnection(config)
	if err != nil {
		log.Printf("[user %s error] NewPeerConnection: %v\n", userId, err)
		return
	}

	// accept one audio}
	_, err = ppc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		log.Printf("[user %s error] AddTransceiverFromKind: %v\n", userId, err)
		return
	}

	// accept one video
	videoTransceiver, err := ppc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		log.Printf("[user %s error] AddTransceiverFromKind: %v\n", userId, err)
		return
	}

	// set codec preference if H264 is required
	if videoCodec == "H264" {
		err = videoTransceiver.SetCodecPreferences(engine.H264Codecs)
		if err != nil {
			log.Printf("[user %s error] SetCodecPreferences: %v\n", userId, err)
			return
		}
	}

	return
}

func newPeerConn(join joinPayload, room *trialRoom, ws *wsConn) (pc *peerConn) {
	userId := join.UserId

	ppc, err := newPionPeerConn(userId, join.VideoCodec)
	if err != nil {
		return
	}

	pc = &peerConn{ppc, make(chan struct{})}

	// trickle ICE. Emit server candidate to client
	pc.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			log.Printf("[user %s error] empty candidate", userId)
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Printf("[user %s error] marshal candidate: %v\n", userId, err)
			return
		}

		ws.SendWithPayload("candidate", string(candidateString))
	})

	// if PeerConnection is closed remove it from global list
	pc.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		log.Printf("[user %s] peer connection state change: %s \n", userId, p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := pc.Close(); err != nil {
				log.Printf("[user %s error] peer connection failed: %v\n", userId, err)
			}
		case webrtc.PeerConnectionStateClosed:
			close(pc.closedCh)
			room.disconnectUser(userId)
		}
	})

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		log.Printf("[user %s] new incoming track: %s\n", userId, remoteTrack.Codec().RTPCodecCapability.MimeType)
		room.incInTracksReadyCount()
		<-room.waitForAllCh

		room.runLocalTrackFromRemote(userId, join, pc, remoteTrack, receiver)
	})

	return
}
