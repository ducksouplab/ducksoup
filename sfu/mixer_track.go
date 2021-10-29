package sfu

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/sequencing"
	"github.com/creamlab/ducksoup/types"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

const (
	defaultInterpolatorStep = 30
	maxInterpolatorDuration = 5000
	statsPeriod             = 3000
)

type mixerTrack struct {
	sync.Mutex
	id                string
	ps                *peerServer
	track             *webrtc.TrackLocalStaticRTP
	pipeline          *gst.Pipeline
	interpolatorIndex map[string]*sequencing.LinearInterpolator
	remoteTrack       *webrtc.TrackRemote
	// stats
	lastStats time.Time
	bits      int64
}

func filePrefixWithCount(join types.JoinPayload, room *trialRoom) string {
	connectionCount := room.joinedCountForUser(join.UserId)
	// time room user count
	return time.Now().Format("20060102-150405.000") +
		"-r-" + join.RoomId +
		"-u-" + join.UserId +
		"-c-" + fmt.Sprint(connectionCount)
}

func parseFx(kind string, join types.JoinPayload) (fx string) {
	if kind == "video" {
		fx = join.VideoFx
	} else {
		fx = join.AudioFx
	}
	return
}

func newMixerTrack(ps *peerServer, remoteTrack *webrtc.TrackRemote) (track *mixerTrack, err error) {
	// create a new mixerTrack with:
	// - the same codec format as the incoming/remote one
	// - a unique server-side trackId, but won't be reused in the browser, see https://developer.mozilla.org/en-US/docs/Web/API/MediaStreamTrack/id
	// - a streamId shared among peerServer tracks (audio/video)
	trackId := uuid.New().String()
	rtpTrack, err := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, trackId, ps.streamId)

	if err != nil {
		return
	}

	track = &mixerTrack{
		id:                remoteTrack.ID(), // reuse of remoteTrack ID
		ps:                ps,
		track:             rtpTrack,
		remoteTrack:       remoteTrack,
		interpolatorIndex: make(map[string]*sequencing.LinearInterpolator),
		lastStats:         time.Now(),
	}
	return
}

func (t *mixerTrack) ID() string {
	return t.track.ID()
}

func (t *mixerTrack) Write(buf []byte) (err error) {
	packet := &rtp.Packet{}
	packet.Unmarshal(buf)
	err = t.track.WriteRTP(packet)

	bits := (packet.MarshalSize() - packet.Header.MarshalSize()) * 8
	t.Lock()
	t.bits += int64(bits)
	t.Unlock()
	return
}

func (t *mixerTrack) loop() {
	join, userId, room, pc := t.ps.join, t.ps.userId, t.ps.room, t.ps.pc

	kind := t.remoteTrack.Kind().String()
	fx := parseFx(kind, join)

	if fx == "forward" {
		// special case for testing: write directly to mixerTrack
		for {
			// Read RTP packets being sent to Pion
			rtp, _, err := t.remoteTrack.ReadRTP()
			if err != nil {
				return
			}
			if err := t.track.WriteRTP(rtp); err != nil {
				return
			}
		}
	} else {
		// main case (with GStreamer): write/push to pipeline which in turn outputs to mixerTrack
		filePrefix := filePrefixWithCount(join, room)
		format := strings.Split(t.remoteTrack.Codec().RTPCodecCapability.MimeType, "/")[1]

		// create and start pipeline
		pliRequestCallback := func() {
			pc.throttledPLIRequest()
		}
		pipeline := gst.CreatePipeline(join, t, kind, format, fx, filePrefix, pliRequestCallback)
		t.pipeline = pipeline

		pipeline.Start()
		room.addFiles(userId, pipeline.Files)
		// stats
		statsTicker := time.NewTicker(statsPeriod * time.Millisecond)
		if t.track.Kind().String() == "video" {
			go func() {
				for tickTime := range statsTicker.C {
					t.Lock()
					milliseconds := tickTime.Sub(t.lastStats).Milliseconds()
					display := fmt.Sprintf("%v kbit/s", t.bits/milliseconds)
					log.Printf("[info] [room#%s] [user#%s] [mixer] video encoded bitrate: %s\n", room.shortId, pc.userId, display)
					t.bits = 0
					t.lastStats = tickTime
					t.Unlock()
				}
			}()
		}

		defer func() {
			log.Printf("[info] [room#%s] [user#%s] [%s track] stopping\n", room.shortId, userId, kind)
			pipeline.Stop()
			statsTicker.Stop()
			if r := recover(); r != nil {
				log.Printf("[recov] [room#%s] [user#%s] [%s track] recover\n", room.shortId, userId, kind)
			}
		}()

		buf := make([]byte, defaultMTU)
		for {
			select {
			case <-room.endCh:
				// trial is over, no need to trigger signaling on every closing track
				return
			case <-t.ps.closedCh:
				// peer may quit early (for instance page refresh), other peers need to be updated
				return
			default:
				i, _, readErr := t.remoteTrack.Read(buf)
				if readErr != nil {
					return
				}
				pipeline.Push(buf[:i])
			}
		}
	}
}

func (t *mixerTrack) controlFx(payload controlPayload) {
	interpolatorId := payload.Kind + payload.Name + payload.Property
	interpolator := t.interpolatorIndex[interpolatorId]

	if interpolator != nil {
		// an interpolation is already running for this pipeline, effect and property
		interpolator.Stop()
	}

	duration := payload.Duration
	if duration == 0 {
		t.pipeline.SetFxProperty(payload.Name, payload.Property, payload.Value)
	} else {
		if duration > maxInterpolatorDuration {
			duration = maxInterpolatorDuration
		}
		oldValue := t.pipeline.GetFxProperty(payload.Name, payload.Property)
		newInterpolator := sequencing.NewLinearInterpolator(oldValue, payload.Value, duration, defaultInterpolatorStep)

		t.Lock()
		t.interpolatorIndex[interpolatorId] = newInterpolator
		t.Unlock()

		defer func() {
			t.Lock()
			delete(t.interpolatorIndex, interpolatorId)
			t.Unlock()
		}()

		for {
			select {
			case <-t.ps.room.endCh:
				return
			case <-t.ps.closedCh:
				return
			case currentValue, more := <-newInterpolator.C:
				if more {
					t.pipeline.SetFxProperty(payload.Name, payload.Property, currentValue)
				} else {
					return
				}
			}
		}
	}
}
