package sfu

const (
	receiveMTU = 1500

	// encoding bitrates in bits/S
	defaultVideoBitrate = 300 * 1000
	minVideoBitrate     = 150 * 1000
	maxVideoBitrate     = 600 * 1000
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
