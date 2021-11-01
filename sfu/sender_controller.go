package sfu

import (
	"io"
	"log"
	"sync"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v3"
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
	// for logging
	roomId     string
	fromUserId string
	toUserId   string
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
		ssrc:           ssrc,
		kind:           kind,
		sender:         sender,
		optimalBitrate: streamConfig.DefaultBitrate,
		maxBitrate:     streamConfig.MaxBitrate,
		roomId:         slice.fromPs.roomId,
		fromUserId:     slice.fromPs.userId,
		toUserId:       toUserId,
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

		log.Printf("[info] [room#%s] [mixer] [from user#%s to user#%s] %d packets lost, previous bitrate %d, new bitrate %d\n",
			sc.roomId, sc.fromUserId, sc.toUserId, loss, prevOptimalBitrate/1000, newOptimalBitrate/1000)
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
					log.Printf("[error] [room#%s] [mixer] [from user#%s to user#%s] reading RTCP: %v\n", sc.roomId, sc.fromUserId, sc.toUserId, err)
					continue
				} else {
					return
				}
			}

			for _, packet := range packets {
				// log.Printf("[info] [room#%s] [mixer] [from user#%s to user#%s] RTCP packet %T\n%v\n", sc.roomId, sc.fromUserId, sc.toUserId, packet, packet)
				switch rtcpPacket := packet.(type) {
				case *rtcp.PictureLossIndication:
					log.Printf("[info] [room#%s] [mixer] [from user#%s to user#%s] PLI received\n", sc.roomId, sc.fromUserId, sc.toUserId)
					sc.slice.fromPs.pc.throttledPLIRequest()
				case *rtcp.ReceiverEstimatedMaximumBitrate:
					// sc.updateRateFromREMB(uint64(rtcpPacket.Bitrate))
					log.Printf("[info] [room#%s] [mixer] [from user#%s to user#%s] REMB packet %T:\n%v\n", sc.roomId, sc.fromUserId, sc.toUserId, rtcpPacket, rtcpPacket)
				case *rtcp.ReceiverReport:
					for _, r := range rtcpPacket.Reports {
						if r.SSRC == uint32(sc.ssrc) {
							sc.updateRateFromLoss(r.FractionLost)
						}
					}
					// default:
					// 	log.Printf("[info] [room#%s] [mixer] [from user#%s to user#%s] RTCP packet %T:\n%v\n", sc.roomId, sc.fromUserId, sc.toUserId, rtcpPacket, rtcpPacket)
				}
			}
		}
	}
}
