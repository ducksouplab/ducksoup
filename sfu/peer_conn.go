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
	connectionCount := room.JoinedCountForUser(joinPayload.UserId)
	// time room user count
	return time.Now().Format("20060102-150405.000") +
		"-r-" + joinPayload.Room +
		"-u-" + joinPayload.UserId + "-" + joinPayload.Name +
		"-c-" + fmt.Sprint(connectionCount)
}

func NewPeerConnection(joinPayload JoinPayload, room *Room, wsConn *WsConn) (peerConn *webrtc.PeerConnection) {

	api, err := NewAPI([]string{"vp8", "opus"})
	if err != nil {
		log.Print(err)
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
	peerConn, err = api.NewPeerConnection(config)
	if err != nil {
		log.Print(err)
		return
	}

	// accept one audio and one video incoming tracks
	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConn.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}

	// trickle ICE. Emit server candidate to client
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

	// if PeerConnection is closed remove it from global list
	peerConn.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		log.Printf("[user #%s] peerConn> state change: %s \n", joinPayload.UserId, p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConn.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			room.UpdateSignaling()
			room.DisconnectUser(joinPayload.UserId)
		}
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("[user #%s] peerConn> new %s track\n", joinPayload.UserId, remoteTrack.Codec().RTPCodecCapability.MimeType)

		buf := make([]byte, 1500)
		room.IncTracksReadyCount()

		<-room.waitForAllCh

		// prepare GStreamer pipeline
		log.Printf("[user #%s] peerConn> %s track started\n", joinPayload.UserId, remoteTrack.Kind().String())
		processedTrack := room.AddProcessedTrack(remoteTrack)
		defer room.RemoveProcessedTrack(processedTrack)

		mediaFilePrefix := filePrefix(joinPayload, room)
		codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]
		pipeline := gst.CreatePipeline(processedTrack, mediaFilePrefix, codecName, joinPayload.Proc)
		pipeline.Start()
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[user #%s] peerConn> recover OnTrack\n", joinPayload.UserId)
			}
		}()
		defer pipeline.Stop()

	processLoop:
		for {
			select {
			case <-room.finishCh:
				wsConn.Send("finish")
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
