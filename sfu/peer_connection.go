package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/creamlab/webrtc-transform/gst"
	"github.com/gouniverse/uid"
	"github.com/pion/webrtc/v3"
)

func NewPeerConnection(room *Room, wsConn *WebsocketConn) (rtcConn *webrtc.PeerConnection) {
	// unique id
	peerUid := uid.HumanUid()

	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	rtcConn, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Print(err)
		return
	}

	// Accept one audio and one video incoming tracks
	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := rtcConn.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}

	// Notify when peer has connected/disconnected
	rtcConn.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// Trickle ICE. Emit server candidate to client
	rtcConn.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}

		if writeErr := wsConn.WriteJSON(&Message{
			Event: "candidate",
			Data:  string(candidateString),
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	// If PeerConnection is closed remove it from global list
	rtcConn.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := rtcConn.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			room.SignalingUpdate()
		}
	})

	rtcConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		buf := make([]byte, 1500)
		codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]

		processedTrack := room.AddProcessedTrack(remoteTrack)
		defer room.RemoveProcessedTrack(processedTrack)

		pipeline := gst.CreatePipeline(peerUid, codecName, processedTrack)
		pipeline.Start()
		defer pipeline.Stop()

		for {
			i, _, readErr := remoteTrack.Read(buf)
			if readErr != nil {
				return
			}
			pipeline.Push(buf[:i])
		}

	})

	// Add our new PeerConnection to room
	room.AddPeer(rtcConn, wsConn)
	room.SignalingUpdate()

	return
}
