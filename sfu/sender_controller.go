package sfu

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/helpers"
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
	slice          *mixerSlice
	fromPs         *peerServer
	toUserId       string
	ssrc           webrtc.SSRC
	kind           string
	sender         *webrtc.RTPSender
	ccEstimator    cc.BandwidthEstimator
	optimalBitrate uint64
	maxBitrate     uint64
	minBitrate     uint64
}

func newSenderController(pc *peerConn, slice *mixerSlice, sender *webrtc.RTPSender) *senderController {
	params := sender.GetParameters()
	kind := slice.output.Kind().String()
	ssrc := params.Encodings[0].SSRC
	streamConfig := config.Video
	if kind == "audio" {
		streamConfig = config.Audio
	}

	return &senderController{
		slice:          slice,
		fromPs:         slice.fromPs,
		toUserId:       pc.userId,
		ssrc:           ssrc,
		kind:           kind,
		sender:         sender,
		ccEstimator:    pc.ccEstimator,
		optimalBitrate: streamConfig.DefaultBitrate,
		maxBitrate:     streamConfig.MaxBitrate,
		minBitrate:     streamConfig.MinBitrate,
	}
}

func (sc *senderController) logError() *zerolog.Event {
	return sc.slice.logError().Str("context", "track").Str("toUser", sc.toUserId)
}

func (sc *senderController) logInfo() *zerolog.Event {
	return sc.slice.logInfo().Str("context", "track").Str("toUser", sc.toUserId)
}

func (sc *senderController) logDebug() *zerolog.Event {
	return sc.slice.logDebug().Str("context", "track").Str("toUser", sc.toUserId)
}

func (sc *senderController) capRate(in uint64) uint64 {
	if in > sc.maxBitrate {
		return sc.maxBitrate
	} else if in < sc.minBitrate {
		return sc.minBitrate
	}
	return in
}

// see https://datatracker.ietf.org/doc/html/draft-ietf-rmcat-gcc-02
// credits to https://github.com/jech/galene
func (sc *senderController) updateRateFromLoss(loss uint8) {
	sc.Lock()
	defer sc.Unlock()

	var newOptimalBitrate uint64
	prevOptimalBitrate := sc.optimalBitrate

	if loss < 5 {
		// loss < 0.02, multiply by 1.05
		newOptimalBitrate = prevOptimalBitrate * 269 / 256
	} else if loss > 25 {
		// loss > 0.1, multiply by (1 - loss/2)
		newOptimalBitrate = prevOptimalBitrate * (512 - uint64(loss)) / 512
		sc.logInfo().Int("value", int(loss)).Msg("loss_threshold_exceeded")
	} else {
		newOptimalBitrate = prevOptimalBitrate
	}

	sc.optimalBitrate = sc.capRate(newOptimalBitrate)
}

func (sc *senderController) loop() {
	estimateWithGCCEnv := helpers.Getenv("DS_GCC") == "true"
	go sc.loopReadRTCP(estimateWithGCCEnv)
	if sc.kind == "video" && estimateWithGCCEnv {
		go sc.loopGCC()
	}
}

func (sc *senderController) loopGCC() {
	ticker := time.NewTicker(gccPeriod * time.Millisecond)
	for {
		select {
		case <-sc.slice.endCh:
			ticker.Stop()
			return
		case <-ticker.C:
			// update optimal video bitrate, leaving room for audio
			sc.Lock()
			sc.optimalBitrate = sc.capRate(uint64(sc.ccEstimator.GetTargetBitrate()) - config.Audio.MaxBitrate)
			sc.Unlock()
			sc.logInfo().Str("target", fmt.Sprintf("%v", sc.ccEstimator.GetTargetBitrate())).Str("stats", fmt.Sprintf("%v", sc.ccEstimator.GetStats())).Msg("gcc")
		}
	}
}
func (sc *senderController) loopReadRTCP(estimateWithGCC bool) {
	for {
		select {
		case <-sc.slice.endCh:
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

			// if bandwidth estimation is done with TWCC+GCC, REMB won't work and RR are not needed
			if estimateWithGCC {
				// only forward PLIs
				for _, packet := range packets {
					switch packet.(type) {
					case *rtcp.PictureLossIndication:
						sc.slice.fromPs.pc.throttledPLIRequest("PLI from other peer")
					}
				}
			} else {
				for _, packet := range packets {
					switch rtcpPacket := packet.(type) {
					case *rtcp.PictureLossIndication:
						sc.slice.fromPs.pc.throttledPLIRequest("PLI from other peer")
					case *rtcp.ReceiverEstimatedMaximumBitrate:
						sc.logDebug().Msgf("%T %+v", packet, packet)
						// sc.updateRateFromREMB(uint64(rtcpPacket.Bitrate))
					case *rtcp.ReceiverReport:
						// sc.logDebug().Msgf("%T %+v", packet, packet)
						for _, r := range rtcpPacket.Reports {
							if r.SSRC == uint32(sc.ssrc) {
								sc.updateRateFromLoss(r.FractionLost)
							}
						}
					}
				}
			}

		}
	}
}
