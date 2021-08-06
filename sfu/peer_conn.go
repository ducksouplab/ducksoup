package sfu

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/engine"
	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/sequencing"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
)

const (
	DefaultWidth            = 800
	DefaultHeight           = 600
	DefaultFrameRate        = 30
	DefaultInterpolatorStep = 30
	MaxInterpolatorDuration = 5000
)

// Augmented pion PeerConnection
type peerConn struct {
	sync.Mutex
	*webrtc.PeerConnection
	room              *trialRoom
	interpolatorIndex map[string]*sequencing.LinearInterpolator
	// if peer connection is closed before room is ended (for instance on browser page refresh)
	closedCh chan struct{}
	// GStreamer references
	audioPipeline *gst.Pipeline
	videoPipeline *gst.Pipeline
}

func filePrefix(joinPayload JoinPayload, room *trialRoom) string {
	connectionCount := room.JoinedCountForUser(joinPayload.UserId)
	// time room user count
	return room.namespace + "/" +
		time.Now().Format("20060102-150405.000") +
		"-r-" + joinPayload.RoomId +
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

func (pc *peerConn) setPipeline(kind string, pipeline *gst.Pipeline) {
	pc.Lock()
	defer pc.Unlock()

	if kind == "audio" {
		pc.audioPipeline = pipeline
	} else if kind == "video" {
		pc.videoPipeline = pipeline
	}
}

// API

func (pc *peerConn) ControlFx(payload ControlPayload) {
	var pipeline *gst.Pipeline
	if payload.Kind == "audio" {
		if pc.audioPipeline == nil {
			return
		}
		pipeline = pc.audioPipeline
	} else if payload.Kind == "video" {
		if pc.videoPipeline == nil {
			return
		}
		pipeline = pc.videoPipeline
	} else {
		return
	}

	interpolatorId := payload.Kind + payload.Name + payload.Property
	interpolator := pc.interpolatorIndex[interpolatorId]

	if interpolator != nil {
		// an interpolation is already running for this pipeline, effect and property
		interpolator.Stop()
	}

	duration := payload.Duration
	if duration == 0 {
		pipeline.SetFxProperty(payload.Name, payload.Property, payload.Value)
	} else {
		if duration > MaxInterpolatorDuration {
			duration = MaxInterpolatorDuration
		}
		oldValue := pipeline.GetFxProperty(payload.Name, payload.Property)
		newInterpolator := sequencing.NewLinearInterpolator(oldValue, payload.Value, duration, DefaultInterpolatorStep)

		pc.Lock()
		pc.interpolatorIndex[interpolatorId] = newInterpolator
		pc.Unlock()

		defer func() {
			pc.Lock()
			delete(pc.interpolatorIndex, interpolatorId)
			pc.Unlock()
		}()

	interpolatorLoop:
		for {
			select {
			case <-pc.room.endCh:
				break interpolatorLoop
			case <-pc.closedCh:
				break interpolatorLoop
			case currentValue, more := <-newInterpolator.C:
				if more {
					pipeline.SetFxProperty(payload.Name, payload.Property, currentValue)
					//log.Println("[interpolate]", payload.Name, payload.Property, currentValue)
				} else {
					break interpolatorLoop
				}
			}
		}
	}

}

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

func NewPeerConn(joinPayload JoinPayload, room *trialRoom, ws *wsConn) (pc *peerConn) {
	userId := joinPayload.UserId

	ppc, err := newPionPeerConn(userId, joinPayload.VideoCodec)
	if err != nil {
		return
	}

	pc = &peerConn{sync.Mutex{}, ppc, room, make(map[string]*sequencing.LinearInterpolator), make(chan struct{}), nil, nil}

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
			room.DisconnectUser(userId)
			room.UpdateSignaling()
		}
	})

	pc.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		// TODO check if needed
		// Send a PLI on an interval so that the publisher is pushing a keyframe every rtcpPLIInterval
		// This is a temporary fix until we implement incoming RTCP events, then we would push a PLI only when a viewer requests it
		go func() {
			ticker := time.NewTicker(time.Second * 3)
			for {
				select {
				case <-room.endCh:
					ticker.Stop()
					return
				case <-pc.closedCh:
					ticker.Stop()
					return
				case <-ticker.C:
					err := pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())}})
					if err != nil {
						log.Printf("[user %s error] WriteRTCP: %v\n", userId, err)
					}
				}
			}
		}()

		log.Printf("[user %s] new track: %s\n", userId, remoteTrack.Codec().RTPCodecCapability.MimeType)

		buf := make([]byte, 1500)
		room.IncTracksReadyCount()

		<-room.waitForAllCh

		// prepare track and room, use the same ids as remoteTrack for simplicity
		processedTrack := room.NewTrack(remoteTrack.Codec().RTPCodecCapability, remoteTrack.ID(), remoteTrack.StreamID())
		log.Printf("[user %s] %s track started\n", userId, remoteTrack.Kind().String())
		defer room.RemoveTrack(remoteTrack.ID())

		kind := remoteTrack.Kind().String()
		fx := parseFx(kind, joinPayload)

		if fx == "forward" {
			// special case for testing: write directly to processedTrack
			for {
				// Read RTP packets being sent to Pion
				rtp, _, err := remoteTrack.ReadRTP()
				if err != nil {
					return
				}
				if err := processedTrack.WriteRTP(rtp); err != nil {
					return
				}
			}
		} else {
			// main case (with GStreamer): write/push to pipeline which in turn outputs to processedTrack
			mediaFilePrefix := filePrefix(joinPayload, room)
			codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]

			// create and start pipeline
			pipeline := gst.CreatePipeline(processedTrack, mediaFilePrefix, kind, codecName, parseWidth(joinPayload), parseHeight(joinPayload), parseFrameRate(joinPayload), parseFx(kind, joinPayload))
			room.BindPipeline(remoteTrack.ID(), pipeline)

			// needed for further interaction from ws to pipeline
			pc.setPipeline(kind, pipeline)

			pipeline.Start()
			room.AddFiles(userId, pipeline.Files)
			defer func() {
				log.Println("stopping", kind)
				pipeline.Stop()
				if r := recover(); r != nil {
					log.Printf("[user %s] recover OnTrack\n", userId)
				}
			}()

		processLoop:
			for {
				select {
				case <-room.endCh:
					break processLoop
				case <-pc.closedCh:
					break processLoop
				default:
					i, _, readErr := remoteTrack.Read(buf)
					if readErr != nil {
						return
					}
					pipeline.Push(buf[:i])
				}
			}
		}

	})

	return
}
