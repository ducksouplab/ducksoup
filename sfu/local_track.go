package sfu

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/sequencing"
	"github.com/google/uuid"
	"github.com/pion/webrtc/v3"
)

type localTrack struct {
	sync.Mutex
	id                string
	ps                *peerServer
	track             *webrtc.TrackLocalStaticRTP
	pipeline          *gst.Pipeline
	interpolatorIndex map[string]*sequencing.LinearInterpolator
	remoteTrack       *webrtc.TrackRemote
}

func filePrefix(join joinPayload, room *trialRoom) string {
	connectionCount := room.joinedCountForUser(join.UserId)
	// time room user count
	return time.Now().Format("20060102-150405.000") +
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

func parseGPU(join joinPayload) bool {
	return join.GPU
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

func newLocalTrack(ps *peerServer, remoteTrack *webrtc.TrackRemote) (track *localTrack, err error) {
	// create a new localTrack with:
	// - the same codec as the incoming/remote one
	// - a unique server-side trackId, but won't be reused in the browser, see https://developer.mozilla.org/en-US/docs/Web/API/MediaStreamTrack/id
	// - a streamId shared among peerServer tracks (audio/video)
	trackId := uuid.New().String()
	rtpTrack, err := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, trackId, ps.streamId)

	if err != nil {
		return
	}

	track = &localTrack{
		id:                remoteTrack.ID(), // reuse of remoteTrack ID
		ps:                ps,
		track:             rtpTrack,
		remoteTrack:       remoteTrack,
		interpolatorIndex: make(map[string]*sequencing.LinearInterpolator),
	}
	return
}

func (l *localTrack) loop() {
	userId, join, room := l.ps.userId, l.ps.join, l.ps.room

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
		mediaFilePrefix := filePrefix(join, room)
		codec := strings.Split(l.remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]

		// create and start pipeline
		pipeline := gst.CreatePipeline(room.shortId, userId, l.track, room.namespace, mediaFilePrefix, kind, codec, parseWidth(join), parseHeight(join), parseFrameRate(join), parseFx(kind, join), parseGPU(join))
		l.pipeline = pipeline

		pipeline.Start()
		room.addFiles(userId, pipeline.Files)
		defer func() {
			log.Printf("[info] [room#%s] [user#%s] [%s track] stopping\n", room.shortId, userId, kind)
			pipeline.Stop()
			if r := recover(); r != nil {
				log.Printf("[recov] [room#%s] [user#%s] [%s track] recover\n", room.shortId, userId, kind)
			}
		}()

		buf := make([]byte, receiveMTU)
		for {
			select {
			case <-room.endCh:
				// trial is over, no need to trigger signaling on every closing track
				return
			case <-l.ps.closedCh:
				// peer may quit early (for instance page refresh), other peers need to be updated
				return
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

		for {
			select {
			case <-l.ps.room.endCh:
				return
			case <-l.ps.closedCh:
				return
			case currentValue, more := <-newInterpolator.C:
				if more {
					l.pipeline.SetFxProperty(payload.Name, payload.Property, currentValue)
				} else {
					return
				}
			}
		}
	}
}
