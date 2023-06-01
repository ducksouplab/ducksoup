package sfu

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/gst"
	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/ducksouplab/ducksoup/sequencing"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

const (
	defaultInterpolatorStep = 30
	maxInterpolatorDuration = 5000
	statsPeriod             = 3000
	diffThreshold           = 10
)

type mixerSlice struct {
	sync.Mutex
	fromPs       *peerServer
	i            *interaction
	kind         string
	streamConfig sfuStream
	// webrtc
	input    *webrtc.TrackRemote
	output   *webrtc.TrackLocalStaticRTP
	receiver *webrtc.RTPReceiver
	// processing
	pipeline          *gst.Pipeline
	interpolatorIndex map[string]*sequencing.LinearInterpolator
	// controller
	senderControllerIndex map[string]*senderController // per user id
	targetBitrate         uint64
	// stats
	lastStats     time.Time
	inputBits     uint64
	outputBits    uint64
	inputBitrate  uint64
	outputBitrate uint64
	// status
	doneCh chan struct{} // stop processing when track is removed
}

// helpers

func minUint64(v []uint64) (min uint64) {
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

// Creates a new mixerSlice with:
// - the same codec format as the incoming/remote one
// - a unique server-side trackId, but won't be reused in the browser, see https://developer.mozilla.org/en-US/docs/Web/API/MediaStreamTrack/id
// - a streamId shared among peerServer tracks (audio/video)
// newId := uuid.New().String()
func newMixerSlice(ps *peerServer, remoteTrack *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) (ms *mixerSlice, err error) {

	kind := remoteTrack.Kind().String()
	var streamConfig sfuStream
	if kind == "video" {
		streamConfig = config.Video
	} else if kind == "audio" {
		streamConfig = config.Audio
	} else {
		err := errors.New("invalid kind")
		ms.logError().Str("context", "track").Err(err).Msg("new_mixer_slice_failed")
		return nil, err
	}

	newId := remoteTrack.ID()
	localTrack, err := webrtc.NewTrackLocalStaticRTP(remoteTrack.Codec().RTPCodecCapability, newId, ps.streamId)

	if err != nil {
		ms.logError().Str("context", "track").Err(err).Msg("new_mixer_slice_failed")
		return
	}

	ms = &mixerSlice{
		fromPs:       ps,
		i:            ps.i,
		kind:         kind,
		streamConfig: streamConfig,
		// webrtc
		input:    remoteTrack,
		output:   localTrack,
		receiver: receiver, // TODO read RTCP?
		// processing
		pipeline:          ps.pipeline,
		interpolatorIndex: make(map[string]*sequencing.LinearInterpolator),
		// controller
		senderControllerIndex: map[string]*senderController{},
		// stats
		lastStats: time.Now(),
		// status
		doneCh: make(chan struct{}),
	}

	return
}

func (ms *mixerSlice) done() chan struct{} {
	return ms.doneCh
}

func (ms *mixerSlice) logError() *zerolog.Event {
	return ms.i.logger.Error().Str("context", "track").Str("user", ms.fromPs.userId)
}

func (ms *mixerSlice) logInfo() *zerolog.Event {
	return ms.i.logger.Info().Str("context", "track").Str("user", ms.fromPs.userId)
}

func (ms *mixerSlice) logDebug() *zerolog.Event {
	return ms.i.logger.Debug().Str("context", "track").Str("user", ms.fromPs.userId)
}

// Same ID as output track
func (ms *mixerSlice) ID() string {
	return ms.output.ID()
}

func (ms *mixerSlice) addSender(pc *peerConn, sender *webrtc.RTPSender) {
	params := sender.GetParameters()

	toUserId := pc.userId
	if len(params.Encodings) == 1 {
		sc := newSenderController(pc, ms, sender)
		ms.Lock()
		ms.senderControllerIndex[toUserId] = sc
		ms.Unlock()
		go sc.loop()
	} else {
		ms.logError().Str("toUser", toUserId).Str("cause", "wrong number of encoding parameters").Msg("add_sender_failed")
	}
}

func (l *mixerSlice) scanInput(buf []byte, n int) {
	packet := &rtp.Packet{}
	packet.Unmarshal(buf)

	l.Lock()
	// estimation (x8 for bytes) not taking int account headers
	// it seems using MarshalSize (like for outputBits below) does not give the right numbers due to packet 0-padding
	l.inputBits += uint64(n) * 8
	l.Unlock()
}

func (ms *mixerSlice) Write(buf []byte) (err error) {
	packet := &rtp.Packet{}
	packet.Unmarshal(buf)
	err = ms.output.WriteRTP(packet)

	if err == nil {
		go func() {
			outputBits := (packet.MarshalSize() - packet.Header.MarshalSize()) * 8
			ms.Lock()
			ms.outputBits += uint64(outputBits)
			ms.Unlock()
		}()
	}

	return
}

func (ms *mixerSlice) close() {
	ms.pipeline.Stop()
	close(ms.doneCh)
	ms.logInfo().Str("track", ms.ID()).Str("kind", ms.kind).Msg("out_track_stopped")
}

func (ms *mixerSlice) loop() {
	defer ms.close()

	pipeline, i, userId := ms.fromPs.pipeline, ms.fromPs.i, ms.fromPs.userId

	// gives pipeline a track to write to
	pipeline.BindTrackAutoStart(ms.kind, ms)
	// wait for audio and video
	<-pipeline.ReadyCh

	// TODO not sure what's best, initializeBitrates or not
	//ms.initializeBitrates()
	i.addFiles(userId, pipeline.OutputFiles()) // for reference
	go ms.runTickers()
	// go ms.runReceiverListener()

	// loop start
	buf := make([]byte, config.Common.MTU)
pushToPipeline:
	for {
		select {
		case <-ms.fromPs.done():
			// interaction OR peer is done
			break pushToPipeline
		default:
			n, _, err := ms.input.Read(buf)
			if err != nil {
				break pushToPipeline
			}
			ms.pipeline.PushRTP(ms.kind, buf[:n])
			// for stats
			go ms.scanInput(buf, n)
		}
	}
}

func (ms *mixerSlice) initializeBitrates() {
	// pipeline is started once (either by the audio or video slice) but both
	// media types need to be initialized*
	ms.pipeline.SetEncodingRate("audio", config.Audio.DefaultBitrate)
	ms.pipeline.SetEncodingRate("video", config.Video.DefaultBitrate)
	// log
	ms.logInfo().Uint64("value", config.Audio.DefaultBitrate/1000).Str("unit", "kbit/s").Msg("audio_target_bitrate_initialized")
	ms.logInfo().Uint64("value", config.Video.DefaultBitrate/1000).Str("unit", "kbit/s").Msg("video_target_bitrate_initialized")
}

func (ms *mixerSlice) updateTargetBitrates(newPotentialRate uint64) {
	ms.Lock()
	ms.targetBitrate = newPotentialRate
	ms.Unlock()
	ms.pipeline.SetEncodingRate(ms.kind, newPotentialRate)
	// format and log
	msg := fmt.Sprintf("%s_target_bitrate_updated", ms.kind)
	ms.logInfo().Uint64("value", newPotentialRate/1000).Str("unit", "kbit/s").Msg(msg)
}

func (ms *mixerSlice) checkOutputBitrate() {
	if ms.kind == "video" {
		ms.Lock()
		if ms.outputBitrate < ms.streamConfig.MinBitrate {
			ms.fromPs.pc.throttledPLIRequest("output_bitrate_is_too_low")
		}
		ms.Unlock()
	}
}

func (ms *mixerSlice) runTickers() {
	// update encoding bitrate on tick and according to minimum controller rate
	go func() {
		encoderTicker := time.NewTicker(time.Duration(config.Common.EncoderControlPeriod) * time.Millisecond)
		defer encoderTicker.Stop()
		for {
			select {
			case <-ms.done():
				return
			case <-encoderTicker.C:
				if len(ms.senderControllerIndex) > 0 {
					rates := []uint64{}
					for _, sc := range ms.senderControllerIndex {
						if env.GCC && ms.kind == "video" {
							rates = append(rates, sc.ccOptimalBitrate)
						} else {
							rates = append(rates, sc.lossOptimalBitrate)
						}
					}
					newPotentialRate := minUint64(rates)
					if ms.pipeline != nil && newPotentialRate > 0 {
						// skip updating previous value and encoding rate too often
						diff := helpers.AbsPercentageDiff(ms.targetBitrate, newPotentialRate)
						// diffIsBigEnough: works also for diff being Inf+ (when updating from 0, diff is Inf+)
						diffIsBigEnough := diff > diffThreshold
						diffToMax := diff > 0 && (newPotentialRate == ms.streamConfig.MaxBitrate)
						if diffIsBigEnough || diffToMax {
							go ms.updateTargetBitrates(newPotentialRate)
						}
					}
				}
			}
		}
	}()

	go func() {
		statsTicker := time.NewTicker(statsPeriod * time.Millisecond)
		defer statsTicker.Stop()
		for {
			select {
			case <-ms.done():
				return
			case tickTime := <-statsTicker.C:
				ms.Lock()
				elapsed := tickTime.Sub(ms.lastStats).Seconds()
				// update bitrates
				ms.inputBitrate = ms.inputBits / uint64(elapsed)
				ms.outputBitrate = ms.outputBits / uint64(elapsed)
				// reset cumulative bits and lastStats
				ms.inputBits = 0
				ms.outputBits = 0
				ms.lastStats = tickTime
				ms.Unlock()
				// may send a PLI if too low
				//ms.checkOutputBitrate()
				// log
				displayInputBitrateKbs := uint64(ms.inputBitrate / 1000)
				displayOutputBitrateKbs := uint64(ms.outputBitrate / 1000)
				displayOutputTargetBitrateKbs := uint64(ms.targetBitrate / 1000)

				inputMsg := fmt.Sprintf("%s_in_bitrate", ms.output.Kind().String())
				outputMsg := fmt.Sprintf("%s_out_bitrate", ms.output.Kind().String())

				ms.logDebug().Uint64("value", displayInputBitrateKbs).Str("unit", "kbit/s").Msg(inputMsg)
				ms.logDebug().Uint64("value", displayOutputBitrateKbs).Uint64("target", displayOutputTargetBitrateKbs).Str("unit", "kbit/s").Msg(outputMsg)
			}
		}
	}()
}

// func (ms *mixerSlice) runReceiverListener() {
// 	buf := make([]byte, defaultMTU)

// 	for {
// 		select {
// 		case <-ms.done():
// 			return
// 		default:
// 			i, _, err := ms.receiver.Read(buf)
// 			if err != nil {
// 				if err != io.EOF && err != io.ErrClosedPipe {
// 					ms.logError().Err(err).Msg("read_received_rtcp_failed")
// 				}
// 				return
// 			}
// 			// TODO: send to rtpjitterbuffer sink_rtcp
// 			//ms.pipeline.PushRTCP(ms.kind, buf[:i])

// 			packets, err := rtcp.Unmarshal(buf[:i])
// 			if err != nil {
// 				ms.logError().Err(err).Msg("unmarshal_received_rtcp_failed")
// 				continue
// 			}

// 			for _, packet := range packets {
// 				switch rtcpPacket := packet.(type) {
// 				case *rtcp.SourceDescription:
// 				// case *rtcp.SenderReport:
// 				// 	log.Println(rtcpPacket)
// 				// case *rtcp.ReceiverEstimatedMaximumBitrate:
// 				// 	log.Println(rtcpPacket)
// 				default:
// 					//ms.logInfo().Msgf("%T %+v", rtcpPacket, rtcpPacket)
// 				}
// 			}
// 		}
// 	}
// }
