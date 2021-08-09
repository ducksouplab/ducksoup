package engine

import (
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
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
	}
	OpusCodecs = []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "audio/opus",
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
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

	m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESMidURI},
		webrtc.RTPCodecTypeVideo,
	)
	m.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESRTPStreamIDURI},
		webrtc.RTPCodecTypeVideo,
	)

	// TODO: investigate why causes image loss after 20 seconds
	// i := &interceptor.Registry{}
	// interesting for instance for NACKs
	// if err := webrtc.RegisterDefaultInterceptors(m, i); err != nil {
	// 	log.Println("[engine error] could not register default interceptors")
	// }

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(m),
		// webrtc.WithInterceptorRegistry(i),
	), nil
}
