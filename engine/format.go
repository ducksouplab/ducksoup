package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ducksouplab/ducksoup/store"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/rs/zerolog"
)

// used for packet dumps

func formatSentRTCP(pkts []rtcp.Packet, _ interceptor.Attributes) (res string) {
	for _, pkt := range pkts {
		switch rtcpPacket := pkt.(type) {
		case *rtcp.TransportLayerNack:
			res += fmt.Sprintf(
				"ssrc:%v type: %T packet: %+v",
				rtcpPacket.MediaSSRC,
				rtcpPacket,
				rtcpPacket,
			)
		case *rtcp.TransportLayerCC:
			res += fmt.Sprintf(
				"ssrc:%v type: %T packet: %+v",
				rtcpPacket.MediaSSRC,
				rtcpPacket,
				rtcpPacket,
			)
		case *rtcp.ReceiverReport:
			for _, report := range rtcpPacket.Reports {
				res += fmt.Sprintf(
					"ssrc:%v type: %T packet: %+v",
					report.SSRC,
					report,
					report,
				)
			}
		default:
			res += fmt.Sprintf("[%T sent] %+v", rtcpPacket, rtcpPacket)
		}
	}
	return res
}

// func formatReceivedRTCP(pkts []rtcp.Packet, _ interceptor.Attributes) (res string) {
// 	for _, pkt := range pkts {
// 		res += fmt.Sprintf("[receiver] %T RTCP packet: %v\n", pkt, pkt)
// 	}
// 	return res
// }

// func formatReceivedRTP(pkt *rtp.Packet, attributes interceptor.Attributes) string {
// 	var twcc rtp.TransportCCExtension
// 	ext := pkt.GetExtension(pkt.GetExtensionIDs()[0])
// 	twcc.Unmarshal(ext)

// 	return fmt.Sprintf("[RTP] #%v now:%v ssrc:%v size:%v",
// 		twcc.TransportSequence,
// 		time.Now().Format("15:04:05.99999999"),
// 		pkt.SSRC,
// 		pkt.MarshalSize(),
// 	)
// }

// used by RTCP log interceptors
type logWriteCloser struct {
	logger zerolog.Logger
}

func (wc *logWriteCloser) Write(p []byte) (n int, err error) {
	n = len(p)
	if n > 0 {
		msg := string(p)
		if strings.HasPrefix(msg, "ssrc") {
			ssrcMatch := ssrcRegexp.FindStringSubmatch(msg)
			// trace level to respect DUCKSOUP_LOG_LEVEL setting
			if len(ssrcMatch) > 0 {
				if ssrc64, err := strconv.ParseUint(ssrcMatch[1], 10, 32); err == nil {
					ssrc := uint32(ssrc64)

					if ssrcLog, ok := store.GetFromSSRCIndex(ssrc); ok {
						// remove ssrc from string and add it as a log prop
						msg = ssrcRegexp.ReplaceAllString(msg, "")
						wc.logger.Trace().
							Str("context", "track").
							Str("ssrc", ssrcMatch[1]).
							Str("namespace", ssrcLog.Namespace).
							Str("interaction", ssrcLog.Interaction).
							Str("user", ssrcLog.User).
							Str("packet", msg).
							Msg(ssrcLog.Kind + "_in_rtcp_emitted")
					}
				}
			}
		} else {
			wc.logger.Trace().Msg(msg)
		}
	}
	return
}

func (wc *logWriteCloser) Close() (err error) {
	return
}
