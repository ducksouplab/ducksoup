package engine

import (
	"log"

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
	VP9Codecs = []webrtc.RTPCodecParameters{
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

// APIs are used to create peer connections, whose preferred codecs are then set once for all (at API level)
// But now preferred codecs are set at transceiver level -> NewWebRTCAPI is from now on unused and may be later removed
func NewWebRTCAPI(videoCodec string) (*webrtc.API, error) {
	if len(videoCodec) < 1 {
		videoCodec = "vp8"
	}
	log.Println("[api] opus and ", videoCodec)
	s := webrtc.SettingEngine{}
	s.SetSRTPReplayProtectionWindow(512)
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	m := webrtc.MediaEngine{}

	// always include opus
	for _, c := range OpusCodecs {
		if err := m.RegisterCodec(c, webrtc.RTPCodecTypeAudio); err != nil {
			return nil, err
		}
	}

	// select video codecs
	if videoCodec == "VP8" {
		for _, c := range VP8Codecs {
			if err := m.RegisterCodec(c, webrtc.RTPCodecTypeVideo); err != nil {
				return nil, err
			}
		}
	} else if videoCodec == "H264" {
		for _, c := range H264Codecs {
			if err := m.RegisterCodec(c, webrtc.RTPCodecTypeVideo); err != nil {
				return nil, err
			}
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

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(&m),
	), nil
}
