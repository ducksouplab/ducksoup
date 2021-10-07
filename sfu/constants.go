package sfu

const (
	receiveMTU = 1500

	// encoding bitrates in bits/S
	defaultVideoBitrate = 500 * 1000
	minVideoBitrate     = 250 * 1000
	maxVideoBitrate     = 1000 * 1000
	defaultAudioBitrate = 48 * 1000
	minAudioBitrate     = 16 * 1000
	maxAudioBitrate     = 64 * 1000

	// video defaults
	defaultWidth     = 800
	defaultHeight    = 600
	defaultFrameRate = 30

	// interpolator default
	defaultInterpolatorStep = 30
	maxInterpolatorDuration = 5000
)
