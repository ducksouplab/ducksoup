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
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
)

const (
	defaultInterpolatorStep = 30
	maxInterpolatorDuration = 5000
	encoderPeriod           = 1000
	statsPeriod             = 3000
	logPeriod               = 7300
)

type mixerSlice struct {
	sync.Mutex
	fromPs *peerServer
	// webrtc
	input    *webrtc.TrackRemote
	output   *webrtc.TrackLocalStaticRTP
	receiver *webrtc.RTPReceiver
	// processing
	pipeline          *gst.Pipeline
	interpolatorIndex map[string]*sequencing.LinearInterpolator
	// controller
	senderControllerIndex map[string]*senderController // per user id
	optimalBitrate        uint64
	encoderTicker         *time.Ticker
	// stats
	statsTicker   *time.Ticker
	logTicker     *time.Ticker
	lastStats     time.Time
	inputBits     int64
	outputBits    int64
	inputBitrate  int64
	outputBitrate int64
	// status
	endCh chan struct{} // stop processing when track is removed
}

// helpers

func minUint64Slice(v []uint64) (min uint64) {
	if len(v) > 0 {
		min = v[0]
	}
	for i := 1; i < len(v); i++ {
		if v[i] < min {
			min = v[i]
		}
	}
	return
}

func filePrefixWithCount(join types.JoinPayload, r *room) string {
	connectionCount := r.joinedCountForUser(join.UserId)
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

func newMixerSlice(ps *peerServer, remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) (slice *mixerSlice, err error) {
	// create a new mixerSlice with:
	// - the same codec format as the incoming/remote one
	// - a unique server-side trackId, but won't be reused in the browser, see https://developer.mozilla.org/en-US/docs/Web/API/MediaStreamTrack/id
	// - a streamId shared among peerServer tracks (audio/video)
	// newId := uuid.New().String()
	newId := remoteTrack.ID()
	localTrack, err := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, newId, ps.streamId)

	if err != nil {
		return
	}

	slice = &mixerSlice{
		fromPs: ps,
		// webrtc
		input:    remoteTrack,
		output:   localTrack,
		receiver: receiver, // TODO read RTCP?
		// processing
		interpolatorIndex: make(map[string]*sequencing.LinearInterpolator),
		// controller
		senderControllerIndex: map[string]*senderController{},
		encoderTicker:         time.NewTicker(encoderPeriod * time.Millisecond),
		// stats
		statsTicker: time.NewTicker(statsPeriod * time.Millisecond),
		logTicker:   time.NewTicker(logPeriod * time.Millisecond),
		lastStats:   time.Now(),
		// status
		endCh: make(chan struct{}),
	}
	return
}

// Same ID as output track
func (s *mixerSlice) ID() string {
	return s.output.ID()
}

func (s *mixerSlice) startTickers() {
	roomId, userId := s.fromPs.r.id, s.fromPs.userId

	// update encoding bitrate on tick and according to minimum controller rate
	go func() {
		for range s.encoderTicker.C {
			if len(s.senderControllerIndex) > 0 {
				rates := []uint64{}
				for _, sc := range s.senderControllerIndex {
					rates = append(rates, sc.optimalBitrate)
				}
				sliceRate := minUint64Slice(rates)
				if s.pipeline != nil && sliceRate > 0 {
					s.Lock()
					s.optimalBitrate = sliceRate
					s.Unlock()
					s.pipeline.SetEncodingRate(sliceRate)
				}
			}
		}
	}()

	go func() {
		for tickTime := range s.statsTicker.C {
			s.Lock()
			elapsed := tickTime.Sub(s.lastStats).Seconds()
			// update bitrates
			s.inputBitrate = s.inputBits / int64(elapsed)
			s.outputBitrate = s.outputBits / int64(elapsed)
			// reset cumulative bits and lastStats
			s.inputBits = 0
			s.outputBits = 0
			s.lastStats = tickTime
			s.Unlock()
			// log
			displayInputBitrateKbs := s.inputBitrate / 1000
			displayOutputBitrateKbs := s.outputBitrate / 1000
			log.Printf("[info] [room#%s] [user#%s] [mixer] %s input bitrate: %v kbit/s\n", roomId, userId, s.output.Kind().String(), displayInputBitrateKbs)
			log.Printf("[info] [room#%s] [user#%s] [mixer] %s output bitrate: %v kbit/s\n", roomId, userId, s.output.Kind().String(), displayOutputBitrateKbs)
		}
	}()

	// periodical log for video
	if s.output.Kind().String() == "video" {
		go func() {
			for range s.logTicker.C {
				display := fmt.Sprintf("%v kbit/s", s.optimalBitrate/1000)
				log.Printf("[info] [room#%s] [user#%s] [mixer] new target bitrate: %s\n", roomId, userId, display)
			}
		}()
	}

}

func (s *mixerSlice) stop() {
	s.pipeline.Stop()
	s.statsTicker.Stop()
	s.encoderTicker.Stop()
	s.logTicker.Stop()
	close(s.endCh)
}

func (s *mixerSlice) addSender(sender *webrtc.RTPSender, toUserId string) {
	roomId := s.fromPs.r.id
	params := sender.GetParameters()

	if len(params.Encodings) == 1 {
		sc := newSenderController(sender, s, toUserId)
		s.Lock()
		s.senderControllerIndex[toUserId] = sc
		s.Unlock()
		go sc.runListener()
	} else {
		log.Printf("[error] [room#%s] [user#%s] [mixer] addSender: wrong number of encoding parameters\n", roomId, toUserId)
	}
}

func (l *mixerSlice) scanInput(buf []byte, n int) {
	packet := &rtp.Packet{}
	packet.Unmarshal(buf)

	l.Lock()
	// estimation (x8 for bytes) not taking int account headers
	// it seems using MarshalSize (like for outputBits below) does not give the right numbers due to packet 0-padding
	l.inputBits += int64(n) * 8
	l.Unlock()
}

func (s *mixerSlice) Write(buf []byte) (err error) {
	packet := &rtp.Packet{}
	packet.Unmarshal(buf)
	err = s.output.WriteRTP(packet)

	if err == nil {
		go func() {
			outputBits := (packet.MarshalSize() - packet.Header.MarshalSize()) * 8
			s.Lock()
			s.outputBits += int64(outputBits)
			s.Unlock()
		}()
	}

	return
}

func (s *mixerSlice) loop() {
	join, room, pc, roomId, userId := s.fromPs.join, s.fromPs.r, s.fromPs.pc, s.fromPs.r.id, s.fromPs.userId
	kind := s.input.Kind().String()
	fx := parseFx(kind, join)

	if fx == "forward" {
		// special case for testing: write directly to mixerSlice
		for {
			// Read RTP packets being sent to Pion
			rtp, _, err := s.input.ReadRTP()
			if err != nil {
				return
			}
			if err := s.output.WriteRTP(rtp); err != nil {
				return
			}
		}
	} else {
		// main case (with GStreamer): write/push to pipeline which in turn outputs to mixerSlice
		filePrefix := filePrefixWithCount(join, room)
		format := strings.Split(s.input.Codec().RTPCodecCapability.MimeType, "/")[1]

		// create and start pipeline
		pliRequestCallback := func() {
			pc.throttledPLIRequest()
		}
		pipeline := gst.CreatePipeline(join, s, kind, format, fx, filePrefix, pliRequestCallback)
		s.pipeline = pipeline

		pipeline.Start()
		room.addFiles(userId, pipeline.Files)
		s.startTickers()

		defer func() {
			log.Printf("[info] [room#%s] [user#%s] [%s track] stopping\n", roomId, userId, kind)
			s.stop()
			if r := recover(); r != nil {
				log.Printf("[recov] [room#%s] [user#%s] [%s track] recover\n", roomId, userId, kind)
			}
		}()

		buf := make([]byte, defaultMTU)
		for {
			select {
			case <-room.endCh:
				// trial is over, no need to trigger signaling on every closing track
				return
			case <-s.fromPs.closedCh:
				// peer may quit early (for instance page refresh), other peers need to be updated
				return
			default:
				i, _, readErr := s.input.Read(buf)
				if readErr != nil {
					return
				}
				pipeline.Push(buf[:i])
				// for stats
				go s.scanInput(buf, i)
			}
		}
	}
}

func (s *mixerSlice) controlFx(payload controlPayload) {
	interpolatorId := payload.Kind + payload.Name + payload.Property
	interpolator := s.interpolatorIndex[interpolatorId]

	if interpolator != nil {
		// an interpolation is already running for this pipeline, effect and property
		interpolator.Stop()
	}

	duration := payload.Duration
	if duration == 0 {
		s.pipeline.SetFxProperty(payload.Name, payload.Property, payload.Value)
	} else {
		if duration > maxInterpolatorDuration {
			duration = maxInterpolatorDuration
		}
		oldValue := s.pipeline.GetFxProperty(payload.Name, payload.Property)
		newInterpolator := sequencing.NewLinearInterpolator(oldValue, payload.Value, duration, defaultInterpolatorStep)

		s.Lock()
		s.interpolatorIndex[interpolatorId] = newInterpolator
		s.Unlock()

		defer func() {
			s.Lock()
			delete(s.interpolatorIndex, interpolatorId)
			s.Unlock()
		}()

		for {
			select {
			case <-s.fromPs.r.endCh:
				return
			case <-s.fromPs.closedCh:
				return
			case currentValue, more := <-newInterpolator.C:
				if more {
					s.pipeline.SetFxProperty(payload.Name, payload.Property, currentValue)
				} else {
					return
				}
			}
		}
	}
}
