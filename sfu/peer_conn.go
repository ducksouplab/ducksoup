package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/creamlab/ducksoup/engine"
	"github.com/creamlab/ducksoup/gst"
	"github.com/pion/webrtc/v3"
)

const (
	DefaultWidth     = 800
	DefaultHeight    = 600
	DefaultFrameRate = 30
)

func filePrefix(joinPayload JoinPayload, room *Room) string {
	connectionCount := room.JoinedCountForUser(joinPayload.UserId)
	// time room user count
	return time.Now().Format("20060102-150405.000") +
		"-r-" + joinPayload.Room +
		"-u-" + joinPayload.UserId +
		"-c-" + fmt.Sprint(connectionCount)
}

func parseFx(kind string, joinPayload JoinPayload) (fx string) {
	if kind == "video" {
		fx = joinPayload.VideoFx
	} else {
		fx = joinPayload.AudioFx
	}
	return
}

func parseWidth(joinPayload JoinPayload) (width int) {
	width = joinPayload.Width
	if width == 0 {
		width = DefaultWidth
	}
	return
}

func parseHeight(joinPayload JoinPayload) (height int) {
	height = joinPayload.Height
	if height == 0 {
		height = DefaultHeight
	}
	return
}

func parseFrameRate(joinPayload JoinPayload) (frameRate int) {
	frameRate = joinPayload.FrameRate
	if frameRate == 0 {
		frameRate = DefaultFrameRate
	}
	return
}

// API

func NewPeerConnection(joinPayload JoinPayload, room *Room, wsConn *WsConn) (peerConn *webrtc.PeerConnection) {
	userId := joinPayload.UserId

	// create RTC API with given set of codecs
	codecs := []string{"opus"}
	if len(joinPayload.VideoCodec) > 0 {
		codecs = append(codecs, joinPayload.VideoCodec)
	} else {
		codecs = append(codecs, "vp8")
	}

	api, err := engine.NewWebRTCAPI(codecs)
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

		wsConn.SendWithPayload("candidate", string(candidateString))
	})

	// if PeerConnection is closed remove it from global list
	peerConn.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		log.Printf("[user #%s] peerConn> state change: %s \n", userId, p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConn.Close(); err != nil {
				log.Print(err)
			}
		case webrtc.PeerConnectionStateClosed:
			room.UpdateSignaling()
			room.DisconnectUser(userId)
		}
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("[user #%s] peerConn> new %s track\n", userId, remoteTrack.Codec().RTPCodecCapability.MimeType)

		buf := make([]byte, 1500)
		room.IncTracksReadyCount()

		<-room.waitForAllCh

		// prepare GStreamer pipeline
		log.Printf("[user #%s] peerConn> %s track started\n", userId, remoteTrack.Kind().String())
		processedTrack := room.AddProcessedTrack(remoteTrack)
		defer room.RemoveProcessedTrack(processedTrack)

		mediaFilePrefix := filePrefix(joinPayload, room)
		codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]

		// prepare pipeline parameters
		kind := remoteTrack.Kind().String()
		// create and start pipeline
		pipeline := gst.CreatePipeline(processedTrack, mediaFilePrefix, kind, codecName, parseWidth(joinPayload), parseHeight(joinPayload), parseFrameRate(joinPayload), parseFx(kind, joinPayload))
		pipeline.Start()
		room.AddFiles(userId, pipeline.Files)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[user #%s] peerConn> recover OnTrack\n", userId)
			}
		}()
		defer pipeline.Stop()

	processLoop:
		for {
			select {
			case <-room.finishCh:
				wsConn.SendWithPayload("finish", room.Files())
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
