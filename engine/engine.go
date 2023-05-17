package engine

// inspired by https://github.com/jech/galene group package

import (
	"regexp"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/pion/ice/v2"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type sfuConfig struct {
	Video struct {
		DefaultBitrate uint64 `yaml:"defaultBitrate"`
	}
}

const (
	portMin = 32768
	portMax = 60999
)

var (
	defaultBitrate    uint64
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
	// get video default encoding bitrate
	var c sfuConfig
	f, err := helpers.Open("config/sfu.yml")
	if err != nil {
		log.Fatal().Err(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&c)
	if err != nil {
		log.Fatal().Err(err)
	}
	defaultBitrate = c.Video.DefaultBitrate

	// other shared vars
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

// APIs are used to create peer connections, possible codecs are set once for all (at API level)
// but preferred codecs for a given track are set at transceiver level
// currently NewWebRTCAPI (rather than pion default one) prevents a freeze/lag observed after ~20 seconds
func NewWebRTCAPI(estimatorCh chan cc.BandwidthEstimator) (*webrtc.API, error) {
	s := webrtc.SettingEngine{}
	s.SetSRTPReplayProtectionWindow(512)
	s.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)
	s.SetEphemeralUDPPortRange(portMin, portMax)

	// initialize media engine
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

	// initialize interceptor registry
	i := &interceptor.Registry{}

	// enhance them
	if err := configureAPIOptions(m, i, estimatorCh); err != nil {
		log.Error().Err(err).Str("context", "peer").Msg("configure_api_failed")
	}

	if len(env.PublicIP) > 0 {
		s.SetNAT1To1IPs([]string{env.PublicIP}, webrtc.ICECandidateTypeHost)
		log.Info().Str("context", "peer").Str("IP", env.PublicIP).Msg("set_host_candidate")
	}

	return webrtc.NewAPI(
		webrtc.WithSettingEngine(s),
		webrtc.WithMediaEngine(m),
		webrtc.WithInterceptorRegistry(i),
	), nil
}
