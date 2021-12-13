package engine

// inspired by https://github.com/jech/galene group package

import (
	"fmt"
	"os"

	_ "github.com/creamlab/ducksoup/helpers" // rely on helpers logger init side-effect
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
	VP9Codecs []webrtc.RTPCodecParameters
)

func init() {
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
			res += fmt.Sprintf(
				"[TWCC] #%v reftime:%v ssrc:%v base:%v count:%v chunks: ",
				rtcpPacket.FbPktCount,
				rtcpPacket.ReferenceTime,
				rtcpPacket.MediaSSRC,
				rtcpPacket.BaseSequenceNumber,
				rtcpPacket.PacketStatusCount,
			)
			for _, chunk := range rtcpPacket.PacketChunks {
				res += fmt.Sprintf("%+v ", chunk)
			}
		case *rtcp.ReceiverReport:
			res += "[RR sent] reports: "
			for _, report := range rtcpPacket.Reports {
				res += fmt.Sprintf("lost=%d/%d ", report.FractionLost, report.TotalLost)
			}
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

	return fmt.Sprintf("[RTP] #%v timestamp:%v ssrc:%v size:%v",
		twcc.TransportSequence,
		pkt.Timestamp,
		pkt.SSRC,
		pkt.MarshalSize(),
	)
}

// used by RTCP log interceptors
type logWriteCloser struct{}

func (wc *logWriteCloser) Write(p []byte) (n int, err error) {
	n = len(p)
	if n > 0 {
		// trace level to respect DS_LOG_LEVEL setting
		log.Logger.Trace().Msg(string(p))
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
		log.Error().Err(err).Msg("[engine] can't register interceptors")
	}

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(m),
		webrtc.WithInterceptorRegistry(i),
	), nil
}
