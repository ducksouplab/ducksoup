package sfu

import (
	"io"
	"sync"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

type senderController struct {
	sync.Mutex
	slice          *mixerSlice
	fromPs         *peerServer
	toUserId       string
	ssrc           webrtc.SSRC
	kind           string
	sender         *webrtc.RTPSender
	optimalBitrate uint64
	maxBitrate     uint64
}

func newSenderController(sender *webrtc.RTPSender, slice *mixerSlice, toUserId string) *senderController {
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
		toUserId:       toUserId,
		ssrc:           ssrc,
		kind:           kind,
		sender:         sender,
		optimalBitrate: streamConfig.DefaultBitrate,
		maxBitrate:     streamConfig.MaxBitrate,
	}
}

func (sc *senderController) logError() *zerolog.Event {
	return sc.slice.logError().Str("context", "track").Str("toUser", sc.toUserId)
}

func (sc *senderController) logInfo() *zerolog.Event {
	return sc.slice.logInfo().Str("context", "track").Str("toUser", sc.toUserId)
}

// see https://datatracker.ietf.org/doc/html/draft-ietf-rmcat-gcc-02
// credits to https://github.com/jech/galene
func (sc *senderController) updateRateFromLoss(loss uint8) {
	sc.Lock()
	defer sc.Unlock()

	var newOptimalBitrate uint64
	prevOptimalBitrate := sc.optimalBitrate

	streamConfig := config.Video
	if sc.kind == "audio" {
		streamConfig = config.Audio
	}

	if loss < 5 {
		// loss < 0.02, multiply by 1.05
		newOptimalBitrate = prevOptimalBitrate * 269 / 256

		if newOptimalBitrate > streamConfig.MaxBitrate {
			newOptimalBitrate = streamConfig.MaxBitrate
		}
	} else if loss > 25 {
		// loss > 0.1, multiply by (1 - loss/2)
		newOptimalBitrate = prevOptimalBitrate * (512 - uint64(loss)) / 512

		if newOptimalBitrate < streamConfig.MinBitrate {
			newOptimalBitrate = streamConfig.MinBitrate
		}

		sc.logInfo().Int("value", int(loss)).Msg("loss_threshold_exceeded")
	} else {
		newOptimalBitrate = prevOptimalBitrate
	}

	if newOptimalBitrate > sc.maxBitrate {
		newOptimalBitrate = sc.maxBitrate
	}
	sc.optimalBitrate = newOptimalBitrate
}

func (sc *senderController) runListener() {
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

			for _, packet := range packets {
				// TODO could implement GCC from TWCC
				switch rtcpPacket := packet.(type) {
				case *rtcp.PictureLossIndication:
					sc.slice.fromPs.pc.throttledPLIRequest("PLI from other peer")
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					// sc.updateRateFromREMB(uint64(rtcpPacket.Bitrate))
				case *rtcp.ReceiverReport:
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
