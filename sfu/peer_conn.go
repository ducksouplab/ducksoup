package sfu

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/creamlab/webrtc-transform/gst"
	"github.com/gouniverse/uid"
	"github.com/pion/webrtc/v3"
)

func NewPeerConnection(room *Room, wsConn *WsConn, userName string) (peerConn *webrtc.PeerConnection) {
	// unique id
	peerUid := uid.HumanUid() + "-" + userName

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConn, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Print(err)
		return
	}

	// Accept one audio and one video incoming tracks
	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConn.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}

	// Notify when peer has connected/disconnected
	peerConn.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		log.Printf("[peerConn] has changed: %s \n", connectionState.String())
	})

	// Trickle ICE. Emit server candidate to client
	peerConn.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}

		if writeErr := wsConn.WriteJSON(&Message{
			Type:    "candidate",
			Payload: string(candidateString),
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	// If PeerConnection is closed remove it from global list
	peerConn.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConn.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			room.SignalingUpdate()
		}
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Println("[peerConn] " + remoteTrack.Kind().String() + " track ready for user " + userName)
		room.readyCh <- struct{}{}
		codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]

		processedTrack := room.AddProcessedTrack(remoteTrack)
		defer room.RemoveProcessedTrack(processedTrack)

		pipeline := gst.CreatePipeline(peerUid, codecName, processedTrack)

		<-room.holdOnCh

		pipeline.Start()
		defer pipeline.Stop()

		buf := make([]byte, 1500)

	loop:
		for {
			select {
			case <-room.stopCh:
				if writeErr := wsConn.WriteJSON(&Message{
					Type:    "stop",
					Payload: "timeout",
				}); writeErr != nil {
					log.Println(writeErr)
				}
				break loop
			default:
				i, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					break loop
				}
				pipeline.Push(buf[:i])
			}
		}

	})

	return
}
