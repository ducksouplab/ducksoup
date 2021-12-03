package sfu

import (
	"io"
	"sync"

	_ "github.com/creamlab/ducksoup/helpers" // rely on helpers logger init side-effect
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type senderController struct {
	sync.Mutex
	slice          *mixerSlice
	fromPs         *peerServer
	ssrc           webrtc.SSRC
	kind           string
	sender         *webrtc.RTPSender
	optimalBitrate uint64
	maxBitrate     uint64
	// log
	logger zerolog.Logger
}

func newSenderController(sender *webrtc.RTPSender, slice *mixerSlice, toUserId string) *senderController {
	params := sender.GetParameters()
	kind := slice.output.Kind().String()
	ssrc := params.Encodings[0].SSRC
	streamConfig := config.Video
	if kind == "audio" {
		streamConfig = config.Audio
	}

	logger := log.With().
		Str("room", slice.fromPs.roomId).
		Str("fromUser", slice.fromPs.userId).
		Str("toUser", toUserId).
		Logger()

	return &senderController{
		slice:          slice,
		fromPs:         slice.fromPs,
		ssrc:           ssrc,
		kind:           kind,
		sender:         sender,
		optimalBitrate: streamConfig.DefaultBitrate,
		maxBitrate:     streamConfig.MaxBitrate,
		logger:         logger,
	}
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

		sc.logger.Info().Msgf("[sender] %d packets lost, previous bitrate %d, new bitrate %d",
			loss, prevOptimalBitrate/1000, newOptimalBitrate/1000)
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
					sc.logger.Error().Err(err).Msg("[sender] can't read RTCP")
					continue
				} else {
					return
				}
			}

			for _, packet := range packets {
				// TODO could implement GCC from TWCC
				switch rtcpPacket := packet.(type) {
				case *rtcp.PictureLossIndication:
					sc.slice.fromPs.pc.throttledPLIRequest()
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
