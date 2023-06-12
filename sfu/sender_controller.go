package sfu

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/config"
	"github.com/ducksouplab/ducksoup/env"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

const (
	gccPeriod = 1000
)

type senderController struct {
	sync.Mutex
	ms                 *mixerSlice
	fromPs             *peerServer
	toUserId           string
	ssrc               webrtc.SSRC
	kind               string
	sender             *webrtc.RTPSender
	ccEstimator        cc.BandwidthEstimator
	ccOptimalBitrate   int
	lossOptimalBitrate int
}

func newSenderController(pc *peerConn, ms *mixerSlice, sender *webrtc.RTPSender) *senderController {
	params := sender.GetParameters()
	kind := ms.output.Kind().String()
	ssrc := params.Encodings[0].SSRC

	// loss-based bitrate estimation is done here
	lossOptimalBitrate := config.SFU.Video.DefaultBitrate
	if kind == "audio" {
		lossOptimalBitrate = config.SFU.Audio.DefaultBitrate
	}
	// ccOptimalBitrate default value is set by ccEstimator

	return &senderController{
		ms:                 ms,
		fromPs:             ms.fromPs,
		toUserId:           pc.userId,
		ssrc:               ssrc,
		kind:               kind,
		sender:             sender,
		ccEstimator:        pc.ccEstimator,
		lossOptimalBitrate: lossOptimalBitrate,
	}
}

func (sc *senderController) logError() *zerolog.Event {
	return sc.ms.logError().Str("context", "track").Str("toUser", sc.toUserId)
}

func (sc *senderController) logInfo() *zerolog.Event {
	return sc.ms.logInfo().Str("context", "track").Str("toUser", sc.toUserId)
}

func (sc *senderController) logDebug() *zerolog.Event {
	return sc.ms.logDebug().Str("context", "track").Str("toUser", sc.toUserId)
}

func (sc *senderController) capRate(in int) int {
	if in > sc.ms.streamConfig.MaxBitrate {
		return sc.ms.streamConfig.MaxBitrate
	} else if in < sc.ms.streamConfig.MinBitrate {
		return sc.ms.streamConfig.MinBitrate
	}
	return in
}

// see https://datatracker.ietf.org/doc/html/draft-ietf-rmcat-gcc-02
// credits to https://github.com/jech/galene
func (sc *senderController) updateRateFromLoss(loss int) {
	sc.Lock()
	defer sc.Unlock()

	var newOptimalBitrate int
	prevOptimalBitrate := sc.lossOptimalBitrate

	if loss < 5 {
		// loss < 0.02, multiply by 1.05
		newOptimalBitrate = prevOptimalBitrate * 269 / 256
	} else if loss > 25 {
		// loss > 0.1, multiply by (1 - loss/2)
		newOptimalBitrate = prevOptimalBitrate * (512 - loss) / 512
		sc.logInfo().Int("value", loss).Msg("loss_threshold_exceeded")
	} else {
		newOptimalBitrate = prevOptimalBitrate
	}

	sc.lossOptimalBitrate = sc.capRate(newOptimalBitrate)
	sc.logInfo().Str("kind", sc.ms.kind).Int("value", sc.lossOptimalBitrate).Msg("loss_optimal_bitrate_updated")
	// plot
	sc.ms.plot.AddSenderLossOptimal(sc.toUserId, sc.lossOptimalBitrate)
}

func (sc *senderController) loop() {
	go sc.loopReadRTCP()

	<-sc.ms.i.ready()
	if sc.kind == "video" && env.GCC {
		// applying GCC only to video is an approximation since audio consumes less bandwidth
		go sc.loopGCC()
	}
}

func (sc *senderController) loopGCC() {
	ticker := time.NewTicker(gccPeriod * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-sc.ms.Done():
			// TODO FIX it could happen that addSender have triggered this loop without the slice
			// to have actually started
			return
		case <-ticker.C:
			sc.Lock()
			// update optimal video bitrate
			// we could leave room for audio and subtracting - config.Audio.MaxBitrate
			sc.ccOptimalBitrate = sc.capRate(sc.ccEstimator.GetTargetBitrate())
			sc.logInfo().Str("kind", sc.ms.kind).Int("value", sc.ccOptimalBitrate).Msg("cc_optimal_bitrate_updated")
			// plot
			sc.ms.plot.AddSenderCCOptimal(sc.toUserId, sc.ccOptimalBitrate)
			sc.Unlock()
			sc.logDebug().Str("target", fmt.Sprintf("%v", sc.ccEstimator.GetTargetBitrate())).Str("stats", fmt.Sprintf("%v", sc.ccEstimator.GetStats())).Msg("gcc")
		}
	}
}
func (sc *senderController) loopReadRTCP() {
	for {
		select {
		case <-sc.ms.Done():
			return
		default:
			packets, _, err := sc.sender.ReadRTCP()
			if err != nil {
				if err != io.EOF && err != io.ErrClosedPipe {
					sc.logError().Err(err).Msg("read_sent_rtcp_failed")
					continue
				} else {
					return
				}
			}

			for _, packet := range packets {
				switch rtcpPacket := packet.(type) {
				case *rtcp.PictureLossIndication:
					sc.ms.fromPs.pc.throttledPLIRequest("forward_from_receiving_peer")
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					sc.logDebug().Msgf("%T %+v", packet, packet)
					// disabled due to TWCC
					// sc.updateRateFromREMB(uint64(rtcpPacket.Bitrate))
				case *rtcp.ReceiverReport:
					// sc.logDebug().Msgf("%T %+v", packet, packet)
					for _, r := range rtcpPacket.Reports {
						if r.SSRC == uint32(sc.ssrc) {
							sc.updateRateFromLoss(int(r.FractionLost))
						}
					}
				}
			}
		}
	}
}
