package sfu

const (
	defaultMTU = 1500

	// works better than VP8 due to vp8 muxer unable to handle stream caps modification (size for instance) and thus
	// requiring decoding and reencoding stream with fixed caps
	defaultVideoFormat = "H264"

	// video defaults
	defaultWidth     = 800
	defaultHeight    = 600
	defaultFrameRate = 30
)
