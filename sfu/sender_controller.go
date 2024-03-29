package sfu

import (
	"fmt"
	"io"
	"sync"

	"github.com/ducksouplab/ducksoup/config"
	"github.com/ducksouplab/ducksoup/env"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
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

func (sc *senderController) logTrace() *zerolog.Event {
	return sc.ms.logTrace().Str("context", "track").Str("toUser", sc.toUserId)
}

func (sc *senderController) capRate(in int) int {
	if in > sc.ms.streamConfig.MaxBitrate {
		return sc.ms.streamConfig.MaxBitrate
	} else if in < sc.ms.streamConfig.MinBitrate {
		return sc.ms.streamConfig.MinBitrate
	}
	return in
}

func (sc *senderController) optimalRate() int {
	if env.GCC {
		return sc.ccOptimalBitrate
	}
	return sc.lossOptimalBitrate
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
	if env.GeneratePlots {
		sc.ms.plot.AddSenderLossOptimal(sc.toUserId, sc.lossOptimalBitrate)
	}
}

func (sc *senderController) loop() {
	if sc.kind == "video" {
		if env.GCC {
			go sc.simpleLoopReadRTCPOnVideo()
		} else {
			go sc.estimateLoopReadRTCPOnVideo()
		}
	} else {
		go sc.loopReadRTCPOnAudio()
	}

	<-sc.ms.i.isStarted()
	if sc.kind == "video" && env.GCC {
		sc.ccEstimator.OnTargetBitrateChange(func(bitrate int) {
			sc.Lock()
			// update optimal video bitrate
			// we could leave room for audio and subtracting - config.Audio.MaxBitrate
			sc.ccOptimalBitrate = sc.capRate(bitrate)
			sc.logInfo().Str("kind", sc.ms.kind).Int("value", sc.ccOptimalBitrate).Msg("cc_optimal_bitrate_updated")
			// plot
			if env.GeneratePlots {
				sc.ms.plot.AddSenderCCOptimal(sc.toUserId, sc.ccOptimalBitrate)
			}
			sc.Unlock()
			sc.logDebug().Str("target", fmt.Sprintf("%v", sc.ccEstimator.GetTargetBitrate())).Str("stats", fmt.Sprintf("%v", sc.ccEstimator.GetStats())).Msg("gcc")
		})
	}
}

func (sc *senderController) loopReadRTCPOnAudio() {
	for {
		select {
		case <-sc.ms.Done():
			return
		default:
			_, _, err := sc.sender.ReadRTCP()
			if err != nil {
				if err != io.EOF && err != io.ErrClosedPipe {
					sc.logError().Err(err).Msg("rtcp_on_sender_failed")
					continue
				} else {
					return
				}
			}
		}
	}
}

func (sc *senderController) estimateLoopReadRTCPOnVideo() {
	for {
		select {
		case <-sc.ms.Done():
			return
		default:
			packets, _, err := sc.sender.ReadRTCP()
			if err != nil {
				if err != io.EOF && err != io.ErrClosedPipe {
					sc.logError().Err(err).Msg("rtcp_on_sender_failed")
					continue
				} else {
					return
				}
			}

			for _, packet := range packets {
				switch rtcpPacket := packet.(type) {
				case *rtcp.PictureLossIndication:
					sc.ms.fromPs.pc.managedPLIRequest("forward_from_receiving_peer")
				// case *rtcp.ReceiverEstimatedMaximumBitrate:
				// disabled due to TWCC
				// sc.updateRateFromREMB(uint64(rtcpPacket.Bitrate))
				case *rtcp.ReceiverReport:
					for _, r := range rtcpPacket.Reports {
						if r.SSRC == uint32(sc.ssrc) {
							sc.updateRateFromLoss(int(r.FractionLost))
						}
					}
				}
				sc.logTrace().Str("type", fmt.Sprintf("%T", packet)).Str("packet", fmt.Sprintf("%+v", packet)).Msg("received_rtcp_on_sender")
			}
		}
	}
}

func (sc *senderController) simpleLoopReadRTCPOnVideo() {
	for {
		select {
		case <-sc.ms.Done():
			return
		default:
			packets, _, err := sc.sender.ReadRTCP()
			if err != nil {
				if err != io.EOF && err != io.ErrClosedPipe {
					sc.logError().Err(err).Msg("rtcp_on_sender_failed")
					continue
				} else {
					return
				}
			}

			for _, packet := range packets {
				switch packet.(type) {
				case *rtcp.PictureLossIndication:
					sc.ms.fromPs.pc.managedPLIRequest("forward_from_receiving_peer")
					sc.ms.pipeline.SendPLI()
					// case *rtcp.ReceiverEstimatedMaximumBitrate:
					// disabled due to TWCC
					// sc.updateRateFromREMB(uint64(rtcpPacket.Bitrate))
				}
				sc.logTrace().Str("type", fmt.Sprintf("%T", packet)).Str("packet", fmt.Sprintf("%+v", packet)).Msg("received_rtcp_on_sender")
			}
		}
	}
}
