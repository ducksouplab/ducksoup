package engine

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/ducksouplab/ducksoup/store"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/packetdump"
	"github.com/pion/interceptor/pkg/report"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// adapted from https://github.com/pion/webrtc/blob/v3.1.2/interceptor.go
func configureAPIOptions(mediaEngine *webrtc.MediaEngine, interceptorRegistry *interceptor.Registry, estimatorCh chan cc.BandwidthEstimator) error {

	if err := configureNack(mediaEngine, interceptorRegistry); err != nil {
		return err
	}

	if err := configureRTCPReports(interceptorRegistry); err != nil {
		return err
	}

	if helpers.Getenv("DS_GCC") == "true" {
		// keep configurations here in that order
		if err := configureEstimator(interceptorRegistry, estimatorCh); err != nil {
			return err
		}
	} else {
		// needed not to block pc
		estimatorCh <- nil
	}

	if err := configureTWCCHeaderExtension(mediaEngine, interceptorRegistry); err != nil {
		return err
	}

	if helpers.Getenv("DS_GEN_TWCC") == "true" {
		if err := configureTWCCSender(mediaEngine, interceptorRegistry); err != nil {
			return err
		}
	}

	if err := configureAbsSendTimeHeaderExtension(mediaEngine); err != nil {
		return err
	}

	if err := configureSDESHeaderExtension(mediaEngine, interceptorRegistry); err != nil {
		return err
	}

	if helpers.Getenv("DS_LOG_LEVEL") == "4" {
		if err := configurePacketDump(interceptorRegistry); err != nil {
			return err
		}
	}

	return nil
}

func configureEstimator(i *interceptor.Registry, estimatorCh chan cc.BandwidthEstimator) error {
	// Create a Congestion Controller. This analyzes inbound and outbound data and provides
	// suggestions on how much we should be sending.
	//
	// Passing `nil` means we use the default Estimation Algorithm which is Google Congestion Control.
	// You can use the other ones that Pion provides, or write your own!
	congestionController, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
		return gcc.NewSendSideBWE(gcc.SendSideBWEInitialBitrate(int(defaultBitrate)))
	})
	congestionController.OnNewPeerConnection(func(id string, ccEstimator cc.BandwidthEstimator) {
		estimatorCh <- ccEstimator
	})

	if err != nil {
		return err
	}

	i.Add(congestionController)
	return nil
}

// ConfigureRTCPReports will setup everything necessary for generating Sender and Receiver Reports
func configureRTCPReports(i *interceptor.Registry) error {
	receiver, err := report.NewReceiverInterceptor()
	if err != nil {
		return err
	}

	sender, err := report.NewSenderInterceptor()
	if err != nil {
		return err
	}

	i.Add(receiver)
	i.Add(sender)
	return nil
}

// ConfigureNack will setup everything necessary for handling generating/responding to nack messages.
func configureNack(m *webrtc.MediaEngine, i *interceptor.Registry) error {
	generator, err := nack.NewGeneratorInterceptor()
	if err != nil {
		return err
	}

	responder, err := nack.NewResponderInterceptor()
	if err != nil {
		return err
	}

	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack"}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack", Parameter: "pli"}, webrtc.RTPCodecTypeVideo)
	i.Add(responder)
	i.Add(generator)
	return nil
}

// ConfigureTWCCHeaderExtensionSender will setup everything necessary for adding
// a TWCC header extension to outgoing RTP packets. This will allow the remote peer to generate TWCC reports.
func configureTWCCHeaderExtension(m *webrtc.MediaEngine, i *interceptor.Registry) error {
	if err := m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, webrtc.RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, webrtc.RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	headerInterceptor, err := twcc.NewHeaderExtensionInterceptor()
	if err != nil {
		return err
	}

	i.Add(headerInterceptor)
	return nil
}

// ConfigureTWCCSender will setup everything necessary for generating TWCC reports.
func configureTWCCSender(m *webrtc.MediaEngine, i *interceptor.Registry) error {
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC}, webrtc.RTPCodecTypeAudio)

	generator, err := twcc.NewSenderInterceptor()
	if err != nil {
		return err
	}

	i.Add(generator)
	return nil
}

// For more accurante REMB reports
func configureAbsSendTimeHeaderExtension(m *webrtc.MediaEngine) error {

	if err := m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.ABSSendTimeURI}, webrtc.RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.ABSSendTimeURI}, webrtc.RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	return nil
}

func configureSDESHeaderExtension(m *webrtc.MediaEngine, i *interceptor.Registry) error {

	if err := m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESMidURI},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESRTPStreamIDURI},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESMidURI},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	if err := m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESRTPStreamIDURI},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	return nil
}

func configurePacketDump(i *interceptor.Registry) error {
	dumper, err := packetdump.NewSenderInterceptor(
		packetdump.RTCPFormatter(formatSentRTCP),
		packetdump.RTCPWriter(&logWriteCloser{}),
		packetdump.RTPWriter(zerolog.Nop()),
	)

	if err != nil {
		return err
	}

	i.Add(dumper)
	return nil
}

// used for packet dumps

func formatSentRTCP(pkts []rtcp.Packet, _ interceptor.Attributes) (res string) {
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

			// case *rtcp.ReceiverReport:
			// 	res += "[RR sent] reports: "
			// 	for _, report := range rtcpPacket.Reports {
			// 		res += fmt.Sprintf("lost=%d/%d ", report.FractionLost, report.TotalLost)
			// 	}
			// default:
			// 	res += fmt.Sprintf("[%T sent]", rtcpPacket)
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

func formatReceivedRTP(pkt *rtp.Packet, attributes interceptor.Attributes) string {
	var twcc rtp.TransportCCExtension
	ext := pkt.GetExtension(pkt.GetExtensionIDs()[0])
	twcc.Unmarshal(ext)

	return fmt.Sprintf("[RTP] #%v now:%v ssrc:%v size:%v",
		twcc.TransportSequence,
		time.Now().Format("15:04:05.99999999"),
		pkt.SSRC,
		pkt.MarshalSize(),
	)
}

// used by RTCP log interceptors
type logWriteCloser struct{}

func (wc *logWriteCloser) Write(p []byte) (n int, err error) {
	n = len(p)
	if n > 0 {
		msg := string(p)
		if strings.HasPrefix(msg, "[TWCC]") {
			ssrcMatch := ssrcRegexp.FindStringSubmatch(msg)
			// trace level to respect DS_LOG_LEVEL setting
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