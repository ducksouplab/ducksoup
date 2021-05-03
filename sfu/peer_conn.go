package sfu

import (
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/creamlab/webrtc-transform/gst"
	"github.com/pion/webrtc/v3"
)

func NewPeerConnection(room *Room, wsConn *WsConn, userName string) (peerConn *webrtc.PeerConnection) {
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

		wsConn.WriteJSON(&Message{
			Type:    "candidate",
			Payload: string(candidateString),
		})
	})

	// If PeerConnection is closed remove it from global list
	peerConn.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		log.Printf("[peerConn] connection state change: %s \n", p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConn.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			room.SignalingUpdate()
			room.PeerQuit()
		}
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Println("[peerConn] " + remoteTrack.Kind().String() + " track ready for user " + userName)
		buf := make([]byte, 1500)
		room.IncTracksReadyCount()

		// Wait for other peers to be ready (read and discard track buffer while waiting)
	waitLoop:
		for {
			select {
			case <-room.waitForAllCh:
				break waitLoop
			default:
				_, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					break waitLoop
				}
			}
		}

		// Prepare GStreamer pipeline
		log.Println("[peerConn] " + remoteTrack.Kind().String() + " track started for user " + userName)
		processedTrack := room.AddProcessedTrack(remoteTrack)
		defer room.RemoveProcessedTrack(processedTrack)

		// Set unique id containing time description till milliseconds and user identifier
		uid := time.Now().Format("20060102-15:04:05.000") + "-" + userName

		codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]
		pipeline := gst.CreatePipeline(uid, codecName, processedTrack)
		pipeline.Start()
		defer pipeline.Stop()

		// Read and process track
	processLoop:
		for {
			select {
			case <-room.stopCh:
				wsConn.WriteJSON(&Message{
					Type:    "stop",
					Payload: "timeout",
				})
				break processLoop
			default:
				i, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					break processLoop
				}
				pipeline.Push(buf[:i])
			}
		}
	})

	return
}
