package engine

import (
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/report"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v3"
)

// adapted from https://github.com/pion/webrtc/blob/v3.1.2/interceptor.go
func registerInterceptors(mediaEngine *webrtc.MediaEngine, interceptorRegistry *interceptor.Registry) error {
	if err := configureNack(mediaEngine, interceptorRegistry); err != nil {
		return err
	}

	if err := configureRTCPReports(interceptorRegistry); err != nil {
		return err
	}

	if err := configureTWCCHeaderExtension(mediaEngine, interceptorRegistry); err != nil {
		return err
	}

	if err := configureTWCCSender(mediaEngine, interceptorRegistry); err != nil {
		return err
	}

	// Right now causes issues with TWCC header extension, and was mainly used for REMB which are not received anymore
	// if err := configureAbsSendTimeHeaderExtension(mediaEngine, interceptorRegistry); err != nil {
	// 	return err
	// }

	// if err := configureSDESHeaderExtension(mediaEngine, interceptorRegistry); err != nil {
	// 	return err
	// }

	return nil
}

// ConfigureRTCPReports will setup everything necessary for generating Sender and Receiver Reports
func configureRTCPReports(interceptorRegistry *interceptor.Registry) error {
	receiver, err := report.NewReceiverInterceptor()
	if err != nil {
		return err
	}

	sender, err := report.NewSenderInterceptor()
	if err != nil {
		return err
	}

	interceptorRegistry.Add(receiver)
	interceptorRegistry.Add(sender)
	return nil
}

// ConfigureNack will setup everything necessary for handling generating/responding to nack messages.
func configureNack(mediaEngine *webrtc.MediaEngine, interceptorRegistry *interceptor.Registry) error {
	generator, err := nack.NewGeneratorInterceptor()
	if err != nil {
		return err
	}

	responder, err := nack.NewResponderInterceptor()
	if err != nil {
		return err
	}

	mediaEngine.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack"}, webrtc.RTPCodecTypeVideo)
	mediaEngine.RegisterFeedback(webrtc.RTCPFeedback{Type: "nack", Parameter: "pli"}, webrtc.RTPCodecTypeVideo)
	interceptorRegistry.Add(responder)
	interceptorRegistry.Add(generator)
	return nil
}

// ConfigureTWCCHeaderExtensionSender will setup everything necessary for adding
// a TWCC header extension to outgoing RTP packets. This will allow the remote peer to generate TWCC reports.
func configureTWCCHeaderExtension(mediaEngine *webrtc.MediaEngine, interceptorRegistry *interceptor.Registry) error {
	if err := mediaEngine.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, webrtc.RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := mediaEngine.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.TransportCCURI}, webrtc.RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	i, err := twcc.NewHeaderExtensionInterceptor()
	if err != nil {
		return err
	}

	interceptorRegistry.Add(i)
	return nil
}

// ConfigureTWCCSender will setup everything necessary for generating TWCC reports.
func configureTWCCSender(mediaEngine *webrtc.MediaEngine, interceptorRegistry *interceptor.Registry) error {
	mediaEngine.RegisterFeedback(webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC}, webrtc.RTPCodecTypeVideo)
	mediaEngine.RegisterFeedback(webrtc.RTCPFeedback{Type: webrtc.TypeRTCPFBTransportCC}, webrtc.RTPCodecTypeAudio)

	generator, err := twcc.NewSenderInterceptor()
	if err != nil {
		return err
	}

	interceptorRegistry.Add(generator)
	return nil
}

// For more accurante REMB reports
func configureAbsSendTimeHeaderExtension(mediaEngine *webrtc.MediaEngine, interceptorRegistry *interceptor.Registry) error {

	if err := mediaEngine.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.ABSSendTimeURI}, webrtc.RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := mediaEngine.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.ABSSendTimeURI}, webrtc.RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	return nil
}

func configureSDESHeaderExtension(mediaEngine *webrtc.MediaEngine, interceptorRegistry *interceptor.Registry) error {

	if err := mediaEngine.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESMidURI},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := mediaEngine.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESRTPStreamIDURI},
		webrtc.RTPCodecTypeVideo,
	); err != nil {
		return err
	}

	if err := mediaEngine.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESMidURI},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	if err := mediaEngine.RegisterHeaderExtension(
		webrtc.RTPHeaderExtensionCapability{URI: sdp.SDESRTPStreamIDURI},
		webrtc.RTPCodecTypeAudio,
	); err != nil {
		return err
	}

	return nil
}
