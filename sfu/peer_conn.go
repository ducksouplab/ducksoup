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
type PeerConn struct {
	sync.Mutex
	*webrtc.PeerConnection
	room              *Room
	interpolatorIndex map[string]*sequencing.LinearInterpolator
	// if peer connection is closed before room is ended (for instance on browser page refresh)
	closedCh chan struct{}
	// GStreamer references
	audioPipeline *gst.Pipeline
	videoPipeline *gst.Pipeline
}

func filePrefix(joinPayload JoinPayload, room *Room) string {
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

func (p *PeerConn) setPipeline(kind string, pipeline *gst.Pipeline) {
	p.Lock()
	defer p.Unlock()

	if kind == "audio" {
		p.audioPipeline = pipeline
	} else if kind == "video" {
		p.videoPipeline = pipeline
	}
}

// API

func (p *PeerConn) ControlFx(payload ControlPayload) {

	log.Println(">>>", payload)
	var pipeline *gst.Pipeline
	if payload.Kind == "audio" {
		if p.audioPipeline == nil {
			return
		}
		pipeline = p.audioPipeline
	} else if payload.Kind == "video" {
		if p.videoPipeline == nil {
			return
		}
		pipeline = p.videoPipeline
	} else {
		return
	}

	interpolatorId := payload.Kind + payload.Name + payload.Property
	interpolator := p.interpolatorIndex[interpolatorId]

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

		p.Lock()
		p.interpolatorIndex[interpolatorId] = newInterpolator
		p.Unlock()

		defer func() {
			p.Lock()
			delete(p.interpolatorIndex, interpolatorId)
			p.Unlock()
		}()

	interpolatorLoop:
		for {
			select {
			case <-p.room.endCh:
				break interpolatorLoop
			case <-p.closedCh:
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
	// configure and create a new RTCPeerConnection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}
	ppc, err = webrtc.NewPeerConnection(config)
	if err != nil {
		log.Printf("[user %s error] NewPeerConnection: %v\n", userId, err)
		return
	}

	// accept one audio
	_, err = ppc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		log.Printf("[user %s error] AddTransceiverFromKind: %v\n", userId, err)
		return
	}

	// accept one video with codec preferences
	videoTransceiver, err := ppc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		log.Printf("[user %s error] AddTransceiverFromKind: %v\n", userId, err)
		return
	}

	// if not, no preference is set
	if videoCodec == "H264" {
		err = videoTransceiver.SetCodecPreferences(engine.H264Codecs)
		if err != nil {
			log.Printf("[user %s error] SetCodecPreferences: %v\n", userId, err)
			return
		}
	}

	return
}

func NewPeerConn(joinPayload JoinPayload, room *Room, wsConn *WsConn) (peerConn *PeerConn) {
	userId := joinPayload.UserId

	pionPeerConnection, err := newPionPeerConn(userId, joinPayload.VideoCodec)
	if err != nil {
		return
	}

	peerConn = &PeerConn{sync.Mutex{}, pionPeerConnection, room, make(map[string]*sequencing.LinearInterpolator), make(chan struct{}), nil, nil}

	// trickle ICE. Emit server candidate to client
	peerConn.OnICECandidate(func(i *webrtc.ICECandidate) {
		if i == nil {
			log.Printf("[user %s error] empty candidate", userId)
			return
		}

		candidateString, err := json.Marshal(i.ToJSON())
		if err != nil {
			log.Printf("[user %s error] marshal candidate: %v\n", userId, err)
			return
		}

		wsConn.SendWithPayload("candidate", string(candidateString))
	})

	// if PeerConnection is closed remove it from global list
	peerConn.OnConnectionStateChange(func(p webrtc.PeerConnectionState) {
		log.Printf("[user %s] peer connection state change: %s \n", userId, p.String())
		switch p {
		case webrtc.PeerConnectionStateFailed:
			if err := peerConn.Close(); err != nil {
				log.Printf("[user %s error] peer connection failed: %v\n", userId, err)
			}
		case webrtc.PeerConnectionStateClosed:
			close(peerConn.closedCh)
			room.DisconnectUser(userId)
			room.UpdateSignaling()
		}
	})

	peerConn.OnTrack(func(remoteTrack *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
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
				case <-peerConn.closedCh:
					ticker.Stop()
					return
				case <-ticker.C:
					err := peerConn.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: uint32(remoteTrack.SSRC())}})
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

		// prepare track and room
		processedTrack := room.AddTrack(remoteTrack)
		log.Printf("[user %s] %s track started\n", userId, remoteTrack.Kind().String())
		defer room.RemoveTrack(processedTrack)

		kind := remoteTrack.Kind().String()
		fx := parseFx(kind, joinPayload)

		if fx == "forward" {
			// special case for testing
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
			// main case: use GStreamer
			mediaFilePrefix := filePrefix(joinPayload, room)
			codecName := strings.Split(remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]

			// create and start pipeline
			pipeline := gst.CreatePipeline(processedTrack, mediaFilePrefix, kind, codecName, parseWidth(joinPayload), parseHeight(joinPayload), parseFrameRate(joinPayload), parseFx(kind, joinPayload))

			// needed for further interaction from ws to pipeline
			peerConn.setPipeline(kind, pipeline)

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
				case <-peerConn.closedCh:
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
