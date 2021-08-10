package sfu

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/sequencing"
	"github.com/pion/webrtc/v3"
)

type localTrack struct {
	sync.Mutex
	id                string
	userId            string
	join              joinPayload
	room              *trialRoom
	pc                *peerConn
	track             *webrtc.TrackLocalStaticRTP
	pipeline          *gst.Pipeline
	interpolatorIndex map[string]*sequencing.LinearInterpolator
	remoteTrack       *webrtc.TrackRemote
}

func filePrefix(join joinPayload, room *trialRoom) string {
	connectionCount := room.joinedCountForUser(join.UserId)
	// time room user count
	return room.namespace + "/" +
		time.Now().Format("20060102-150405.000") +
		"-r-" + join.RoomId +
		"-u-" + join.UserId +
		"-c-" + fmt.Sprint(connectionCount)
}

func parseFx(kind string, join joinPayload) (fx string) {
	if kind == "video" {
		fx = join.VideoFx
	} else {
		fx = join.AudioFx
	}
	return
}

func parseWidth(join joinPayload) (width int) {
	width = join.Width
	if width == 0 {
		width = defaultWidth
	}
	return
}

func parseHeight(join joinPayload) (height int) {
	height = join.Height
	if height == 0 {
		height = defaultHeight
	}
	return
}

func parseFrameRate(join joinPayload) (frameRate int) {
	frameRate = join.FrameRate
	if frameRate == 0 {
		frameRate = defaultFrameRate
	}
	return
}

func newLocalTrack(userId string, room *trialRoom, join joinPayload, pc *peerConn, remoteTrack *webrtc.TrackRemote) (track *localTrack, err error) {
	// Create a new TrackLocal with the same codec as the incoming one
	rtpTrack, err := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, remoteTrack.ID(), remoteTrack.StreamID())

	if err != nil {
		log.Printf("[user %s error] NewTrackLocalStaticRTP: %v\n", userId, err)
		return
	}

	track = &localTrack{
		id:                remoteTrack.ID(), // reuse of remoteTrack ID
		userId:            userId,
		join:              join,
		room:              room,
		pc:                pc,
		track:             rtpTrack,
		remoteTrack:       remoteTrack,
		interpolatorIndex: make(map[string]*sequencing.LinearInterpolator),
	}
	return
}

func (l *localTrack) loop() {
	join := l.join
	kind := l.remoteTrack.Kind().String()
	fx := parseFx(kind, join)

	if fx == "forward" {
		// special case for testing: write directly to localTrack
		for {
			// Read RTP packets being sent to Pion
			rtp, _, err := l.remoteTrack.ReadRTP()
			if err != nil {
				return
			}
			if err := l.track.WriteRTP(rtp); err != nil {
				return
			}
		}
	} else {
		// main case (with GStreamer): write/push to pipeline which in turn outputs to localTrack
		mediaFilePrefix := filePrefix(join, l.room)
		codec := strings.Split(l.remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]

		// create and start pipeline
		pipeline := gst.CreatePipeline(l.track, mediaFilePrefix, kind, codec, parseWidth(join), parseHeight(join), parseFrameRate(join), parseFx(kind, join))
		l.pipeline = pipeline

		pipeline.Start()
		l.room.addFiles(l.userId, pipeline.Files)
		defer func() {
			log.Printf("[user %s] stopping %s\n", l.userId, kind)
			pipeline.Stop()
			if r := recover(); r != nil {
				log.Printf("[user %s] recover OnTrack\n", l.userId)
			}
		}()

		buf := make([]byte, receiveMTU)
	processLoop:
		for {
			select {
			case <-l.room.endCh:
				break processLoop
			case <-l.pc.closedCh:
				break processLoop
			default:
				i, _, readErr := l.remoteTrack.Read(buf)
				if readErr != nil {
					return
				}
				pipeline.Push(buf[:i])
			}
		}
	}
}

func (l *localTrack) controlFx(payload controlPayload) {
	interpolatorId := payload.Kind + payload.Name + payload.Property
	interpolator := l.interpolatorIndex[interpolatorId]

	if interpolator != nil {
		// an interpolation is already running for this pipeline, effect and property
		interpolator.Stop()
	}

	duration := payload.Duration
	if duration == 0 {
		l.pipeline.SetFxProperty(payload.Name, payload.Property, payload.Value)
	} else {
		if duration > maxInterpolatorDuration {
			duration = maxInterpolatorDuration
		}
		oldValue := l.pipeline.GetFxProperty(payload.Name, payload.Property)
		newInterpolator := sequencing.NewLinearInterpolator(oldValue, payload.Value, duration, defaultInterpolatorStep)

		l.Lock()
		l.interpolatorIndex[interpolatorId] = newInterpolator
		l.Unlock()

		defer func() {
			l.Lock()
			delete(l.interpolatorIndex, interpolatorId)
			l.Unlock()
		}()

	interpolatorLoop:
		for {
			select {
			case <-l.room.endCh:
				break interpolatorLoop
			case <-l.pc.closedCh:
				break interpolatorLoop
			case currentValue, more := <-newInterpolator.C:
				if more {
					l.pipeline.SetFxProperty(payload.Name, payload.Property, currentValue)
					//log.Println("[interpolate]", payload.Name, payload.Property, currentValue)
				} else {
					break interpolatorLoop
				}
			}
		}
	}
}
