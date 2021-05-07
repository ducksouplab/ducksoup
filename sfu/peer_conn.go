package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/creamlab/webrtc-transform/gst"
	"github.com/pion/webrtc/v3"
)

func filePrefix(joinPayload JoinPayload, room *Room) string {
	connectionCount := room.UserJoinedCount(joinPayload.UserId)
	// time room user count
	return time.Now().Format("20060102-150405.000") +
		"-r-" + joinPayload.Room +
		"-u-" + joinPayload.UserId + "-" + joinPayload.Name +
		"-c-" + fmt.Sprint(connectionCount)
}

func NewPeerConnection(joinPayload JoinPayload, room *Room, wsConn *WsConn) (peerConn *webrtc.PeerConnection) {
	userDisplay := joinPayload.UserId + "-" + joinPayload.Name
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

		wsConn.SendJSON(&Message{
			Type:    "candidate",
			Payload: string(candidateString),
		})
	})

	// If PeerConnection is closed remove it from global list
	peerConn.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		log.Printf("[user #%s] peerConn> state change: %s \n", userDisplay, p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConn.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			room.SignalingUpdate()
			room.RemovePeer(joinPayload.UserId)
		}
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("[user #%s] peerConn> new %s track \n", userDisplay, remoteTrack.Kind().String())
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
					return
				}
			}
		}

		// Prepare GStreamer pipeline
		log.Printf("[user #%s] peerConn> %s track started\n", userDisplay, remoteTrack.Kind().String())
		processedTrack := room.AddProcessedTrack(remoteTrack)
		defer room.RemoveProcessedTrack(processedTrack)

		mediaFilePrefix := filePrefix(joinPayload, room)
		codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]
		pipeline := gst.CreatePipeline(mediaFilePrefix, codecName, joinPayload.Proc, processedTrack)
		pipeline.Start()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[user #%s] peerConn> recover OnTrack\n", userDisplay)
			}
		}()
		defer pipeline.Stop()

		// Read and process track
	processLoop:
		for {
			select {
			case <-room.stopCh:
				wsConn.Send("stop")
				break processLoop
			default:
				i, _, readErr := remoteTrack.Read(buf)
				if readErr != nil {
					return
				}
				pipeline.Push(buf[:i])
			}
		}
	})

	return
}
