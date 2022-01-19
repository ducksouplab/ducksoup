package engine

// inspired by https://github.com/jech/galene group package

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/creamlab/ducksoup/helpers" // rely on helpers logger init side-effect
	"github.com/creamlab/ducksoup/store"
	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/packetdump"
	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	videoRTCPFeedback []webrtc.RTCPFeedback
	// exported
	OpusCodecs []webrtc.RTPCodecParameters
	H264Codecs []webrtc.RTPCodecParameters
	VP8Codecs  []webrtc.RTPCodecParameters
	// VP9 is not supported for the time being (GStreamer pipelines remained to be defined)
	VP9Codecs                           []webrtc.RTPCodecParameters
	ssrcRegexp, countRegexp, lostRegexp *regexp.Regexp
)

func init() {
	ssrcRegexp = regexp.MustCompile(`ssrc:(.*?) `)
	countRegexp = regexp.MustCompile(`count:(.*?) `)
	lostRegexp = regexp.MustCompile(`lost:(.*?)$`)
	videoRTCPFeedback = []webrtc.RTCPFeedback{
		{Type: "goog-remb", Parameter: ""},
		{Type: "ccm", Parameter: "fir"},
		{Type: "nack", Parameter: ""},
		{Type: "nack", Parameter: "pli"},
		{Type: "transport-cc", Parameter: ""},
	}
	OpusCodecs = []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "audio/opus",
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1;stereo=0",
				RTCPFeedback: nil},
			PayloadType: 111,
		},
	}
	H264Codecs = []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/H264",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 102,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/H264",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 127,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/H264",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 125,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/H264",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42e01f",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 108,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/H264",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 123,
		},
	}
	VP8Codecs = []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/VP8",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 96,
		},
	}
	VP9Codecs = []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/VP9",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "profile-id=0",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 98,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "video/VP9",
				ClockRate:    90000,
				Channels:     0,
				SDPFmtpLine:  "profile-id=1",
				RTCPFeedback: videoRTCPFeedback},
			PayloadType: 100,
		},
	}
}

// func formatReceivedRTCP(pkts []rtcp.Packet, _ interceptor.Attributes) (res string) {
// 	for _, pkt := range pkts {
// 		res += fmt.Sprintf("[receiver] %T RTCP packet: %v\n", pkt, pkt)
// 	}
// 	return res
// }

func formatSentRTCP(pkts []rtcp.Packet, _ interceptor.Attributes) (res string) {
	for _, pkt := range pkts {
		switch rtcpPacket := pkt.(type) {
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
										Str("room", ssrcLog.Room).
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

// APIs are used to create peer connections, possible codecs are set once for all (at API level)
// but preferred codecs for a given track are set at transceiver level
// currently NewWebRTCAPI (rather than pion default one) prevents a freeze/lag observed after ~20 seconds
func NewWebRTCAPI() (*webrtc.API, error) {
	s := webrtc.SettingEngine{}
	s.SetSRTPReplayProtectionWindow(512)
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	m := &webrtc.MediaEngine{}

	// always include opus
	for _, c := range OpusCodecs {
		if err := m.RegisterCodec(c, webrtc.RTPCodecTypeAudio); err != nil {
			return nil, err
		}
	}

	// select video codecs
	for _, c := range VP8Codecs {
		if err := m.RegisterCodec(c, webrtc.RTPCodecTypeVideo); err != nil {
			return nil, err
		}
	}
	for _, c := range H264Codecs {
		if err := m.RegisterCodec(c, webrtc.RTPCodecTypeVideo); err != nil {
			return nil, err
		}
	}

	i := &interceptor.Registry{}

	if os.Getenv("DS_LOG_LEVEL") == "4" {
		// logReceived, _ := packetdump.NewReceiverInterceptor(
		// 	packetdump.RTCPWriter(zerolog.Nop()),
		// 	packetdump.RTPFormatter(formatReceivedRTP),
		// 	packetdump.RTPWriter(&logWriteCloser{}),
		// )
		// i.Add(logReceived)

		logSent, _ := packetdump.NewSenderInterceptor(
			packetdump.RTCPFormatter(formatSentRTCP),
			packetdump.RTCPWriter(&logWriteCloser{}),
			packetdump.RTPWriter(zerolog.Nop()),
		)
		i.Add(logSent)
	}
	if err := registerInterceptors(m, i); err != nil {
		log.Error().Err(err).Str("context", "peer").Msg("engine can't register interceptors")
	}

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(m),
		webrtc.WithInterceptorRegistry(i),
	), nil
}
