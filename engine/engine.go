package engine

import (
	"log"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/pion/ice/v2"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

func NewWebRTCAPI(names []string) (*webrtc.API, error) {
	log.Println("api ", names)
	s := webrtc.SettingEngine{}
	s.SetSRTPReplayProtectionWindow(512)
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	m := webrtc.MediaEngine{}

	if err := m.RegisterCodec(
		webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:     "audio/opus",
				ClockRate:    48000,
				Channels:     2,
				SDPFmtpLine:  "minptime=10;useinbandfec=1;stereo=1;sprop-stereo=1",
				RTCPFeedback: nil},
			PayloadType: 111,
		},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return nil, err
	}

	videoRTCPFeedback := []webrtc.RTCPFeedback{
		{Type: "goog-remb", Parameter: ""},
		{Type: "ccm", Parameter: "fir"},
		{Type: "nack", Parameter: ""},
		{Type: "nack", Parameter: "pli"},
	}

	if helpers.Contains(names, "vp8") {
		if err := m.RegisterCodec(
			webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/VP8",
					ClockRate:    90000,
					Channels:     0,
					SDPFmtpLine:  "",
					RTCPFeedback: videoRTCPFeedback},
				PayloadType: 96,
			},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, err
		}
	}

	if helpers.Contains(names, "vp9") {
		if err := m.RegisterCodec(
			webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/VP9",
					ClockRate:    90000,
					Channels:     0,
					SDPFmtpLine:  "profile-id=0",
					RTCPFeedback: videoRTCPFeedback},
				PayloadType: 98,
			},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, err
		}

		if err := m.RegisterCodec(
			webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/VP9",
					ClockRate:    90000,
					Channels:     0,
					SDPFmtpLine:  "profile-id=1",
					RTCPFeedback: videoRTCPFeedback},
				PayloadType: 100,
			},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, err
		}
	}

	if helpers.Contains(names, "h264") {
		if err := m.RegisterCodec(
			webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/H264",
					ClockRate:    90000,
					Channels:     0,
					SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
					RTCPFeedback: videoRTCPFeedback},
				PayloadType: 102,
			},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, err
		}

		if err := m.RegisterCodec(
			webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/H264",
					ClockRate:    90000,
					Channels:     0,
					SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42e01f",
					RTCPFeedback: videoRTCPFeedback},
				PayloadType: 108,
			},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, err
		}

		if err := m.RegisterCodec(
			webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/H264",
					ClockRate:    90000,
					Channels:     0,
					SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032",
					RTCPFeedback: videoRTCPFeedback},
				PayloadType: 123,
			},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, err
		}

		if err := m.RegisterCodec(
			webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/H264",
					ClockRate:    90000,
					Channels:     0,
					SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
					RTCPFeedback: videoRTCPFeedback},
				PayloadType: 125,
			},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
			return nil, err
		}

		if err := m.RegisterCodec(
			webrtc.RTPCodecParameters{
				RTPCodecCapability: webrtc.RTPCodecCapability{
					MimeType:     "video/H264",
					ClockRate:    90000,
					Channels:     0,
					SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f",
					RTCPFeedback: videoRTCPFeedback},
				PayloadType: 127,
			},
			webrtc.RTPCodecTypeVideo,
		); err != nil {
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

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(&m),
	), nil
}
