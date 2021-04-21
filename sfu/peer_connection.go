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

func NewPeerConnection(conn *Conn) (peerConnection *webrtc.PeerConnection) {
	// Prepare the configuration
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	// Create a new RTCPeerConnection
	peerConnection, err := webrtc.NewPeerConnection(config)
	if err != nil {
		log.Print(err)
		return
	}

	// Accept one audio and one video incoming tracks
	for _, typ := range []webrtc.RTPCodecType{webrtc.RTPCodecTypeVideo, webrtc.RTPCodecTypeAudio} {
		if _, err := peerConnection.AddTransceiverFromKind(typ, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionRecvonly,
		}); err != nil {
			log.Print(err)
			return
		}
	}

	// Notify when peer has connected/disconnected
	peerConnection.OnICEConnectionStateChange(func(connectionState webrtc.ICEConnectionState) {
		fmt.Printf("Connection State has changed %s \n", connectionState.String())
	})

	// Trickle ICE. Emit server candidate to client
	peerConnection.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Println(err)
			return
		}

		if writeErr := conn.WriteJSON(&websocketMessage{
			Event: "candidate",
			Data:  string(candidateString),
		}); writeErr != nil {
			log.Println(writeErr)
		}
	})

	// If PeerConnection is closed remove it from global list
	peerConnection.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConnection.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			signalingUpdate()
		}
	})

	peerConnection.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		buf := make([]byte, 1500)
		codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]
		uid := uid.HumanUid()

		var localTrack *webrtc.TrackLocalStaticRTP
		if remoteTrack.Kind().String() == "audio" {
			localTrack = addAudioTrack(remoteTrack)
			defer removeAudioTrack(localTrack)
		} else {
			localTrack = addVideoTrack(remoteTrack)
			defer removeVideoTrack(localTrack)
		}

		pipeline := gst.CreatePipeline(uid, codecName, localTrack)
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

	return
}
