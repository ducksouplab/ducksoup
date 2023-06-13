package engine

import (
	"time"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
	"github.com/pion/interceptor/pkg/packetdump"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
	"github.com/rs/zerolog"
)

// adapted from https://github.com/pion/webrtc/blob/v3.2.8/interceptor.go
func configureAPIOptions(m *webrtc.MediaEngine, r *interceptor.Registry, estimatorCh chan cc.BandwidthEstimator) error {
	// order matters!
	if env.LogLevel == 4 {
		if err := configurePacketDump(r); err != nil {
			return err
		}
	}

	if err := webrtc.ConfigureNack(m, r); err != nil {
		return err
	}

	if err := webrtc.ConfigureRTCPReports(r); err != nil {
		return err
	}

	if env.GCC {
		// keep configurations here in that order
		if err := configureEstimator(r, estimatorCh); err != nil {
			return err
		}
	} else {
		// needed not to block pc
		estimatorCh <- nil
	}

	if err := webrtc.ConfigureTWCCHeaderExtensionSender(m, r); err != nil {
		return err
	}

	if env.GenerateTWCC {
		if err := configureTWCCSender(m, r); err != nil {
			return err
		}
	}

	if err := configureAbsSendTimeHeaderExtension(m); err != nil {
		return err
	}

	if err := configureSDESHeaderExtension(m); err != nil {
		return err
	}

	return nil
}

func configureEstimator(r *interceptor.Registry, estimatorCh chan cc.BandwidthEstimator) error {
	// Create a Congestion Controller. This analyzes inbound and outbound data and provides
	// suggestions on how much we should be sending.
	//
	// Passing `nil` means we use the default Estimation Algorithm which is Google Congestion Control.
	// You can use the other ones that Pion provides, or write your own!
	congestionController, err := cc.NewInterceptor(func() (cc.BandwidthEstimator, error) {
		return gcc.NewSendSideBWE(
			gcc.SendSideBWEInitialBitrate(defaultBitrate),
			gcc.SendSideBWEMaxBitrate(maxBitrate),
			gcc.SendSideBWEMinBitrate(minBitrate),
		)
	})
	congestionController.OnNewPeerConnection(func(id string, ccEstimator cc.BandwidthEstimator) {
		estimatorCh <- ccEstimator
	})

	if err != nil {
		return err
	}

	r.Add(congestionController)
	return nil
}

// ConfigureTWCCSender will setup everything necessary for generating TWCC reports.
func configureTWCCSender(m *webrtc.MediaEngine, r *interceptor.Registry) error {
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC}, webrtc.RTPCodecTypeVideo)
	m.RegisterFeedback(webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC}, webrtc.RTPCodecTypeAudio)

	generator, err := twcc.NewSenderInterceptor(twcc.SendInterval(30 * time.Millisecond))
	if err != nil {
		return err
	}

	r.Add(generator)
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

func configureSDESHeaderExtension(m *webrtc.MediaEngine) error {

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

func configurePacketDump(r *interceptor.Registry) error {
	dumper, err := packetdump.NewSenderInterceptor(
		packetdump.RTCPFormatter(formatSentRTCP),
		packetdump.RTCPWriter(&logWriteCloser{}),
		packetdump.RTPWriter(zerolog.Nop()),
	)

	if err != nil {
		return err
	}

	r.Add(dumper)
	return nil
}
