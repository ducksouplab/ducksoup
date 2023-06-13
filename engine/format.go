package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ducksouplab/ducksoup/store"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/rs/zerolog/log"
)

// used for packet dumps

func rtcpFormatSent(pkts []rtcp.Packet, _ interceptor.Attributes) (res string) {
	for _, pkt := range pkts {
		switch rtcpPacket := pkt.(type) {
		case *rtcp.TransportLayerNack:
			res += fmt.Sprintf(
				"[NACK] ssrc:%v sender:%v nacks:%v",
				rtcpPacket.MediaSSRC,
				rtcpPacket.SenderSSRC,
				rtcpPacket.Nacks,
			)
		case *rtcp.TransportLayerCC:
			var count uint16 = 0
			var lost uint16 = 0
			for _, chunk := range rtcpPacket.PacketChunks {
				switch chk := chunk.(type) {
				case *rtcp.RunLengthChunk:
					count += chk.RunLength
					if chk.PacketStatusSymbol == 0 {
						lost += chk.RunLength
					}
				case *rtcp.StatusVectorChunk:
					for _, symbol := range chk.SymbolList {
						count += 1
						if symbol == 0 {
							lost += 1
						}
					}
				}
			}
			res += fmt.Sprintf(
				"[TWCC] ssrc:%v count:%v lost:%v",
				rtcpPacket.MediaSSRC,
				count,
				lost,
			)

		case *rtcp.ReceiverReport:
			res += "[RR sent] reports: "
			for _, report := range rtcpPacket.Reports {
				res += fmt.Sprintf("lost=%d/%d ", report.FractionLost, report.TotalLost)
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
type logWriteCloser struct{}

func (wc *logWriteCloser) Write(p []byte) (n int, err error) {
	n = len(p)
	if n > 0 {
		msg := string(p)
		if strings.HasPrefix(msg, "[TWCC]") {
			ssrcMatch := ssrcRegexp.FindStringSubmatch(msg)
			// trace level to respect DUCKSOUP_LOG_LEVEL setting
			if len(ssrcMatch) > 0 {
				// remove ssrc from string and add it as a log prop
				msg = ssrcRegexp.ReplaceAllString(msg, "")

				if ssrc64, err := strconv.ParseUint(ssrcMatch[1], 10, 32); err == nil {
					ssrc := uint32(ssrc64)
					ssrcLog := store.GetFromSSRCIndex(ssrc)
					if ssrcLog != nil {
						countMatch := countRegexp.FindStringSubmatch(msg)
						lostMatch := lostRegexp.FindStringSubmatch(msg)

						if len(countMatch) > 0 && len(lostMatch) > 0 {
							// don't log empty counts
							if count, err := strconv.ParseUint(countMatch[1], 10, 64); err == nil && count != 0 {
								if lost, err := strconv.ParseUint(lostMatch[1], 10, 64); err == nil {
									log.Logger.Trace().
										Str("context", "track").
										Str("ssrc", ssrcMatch[1]).
										Str("namespace", ssrcLog.Namespace).
										Str("interaction", ssrcLog.Interaction).
										Str("user", ssrcLog.User).
										Uint64("lost", lost).
										Uint64("count", count).
										Msg(ssrcLog.Kind + "_in_report")
								}
							}
						}
					}
				}
			}
		} else {
			log.Logger.Trace().Msg(msg)
		}
	}
	return
}

func (wc *logWriteCloser) Close() (err error) {
	return
}
