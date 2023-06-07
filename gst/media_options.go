package gst

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ducksouplab/ducksoup/config"
)

// capitalized props are accessible to template
type mediaOptions struct {
	// calculated property
	SkipFixedCaps bool
	// live property depending on Join (and DUCKSOUP_NVCODEC), not used within template
	nvCodec bool
	nvCuda  bool
	Overlay bool
	// properties depending on yml definitions
	DefaultBitrate  uint64
	DefaultKBitrate uint64
	Fx              string
	Muxer           string
	Extension       string
	Decoder         string
	Encoder         string
	Rtp             struct {
		Caps         string
		Pay          string
		Depay        string
		JitterBuffer string
	}
}

func (mo *mediaOptions) addSharedAudioProperties() {
	// used in template or by template helpers
	mo.Rtp.JitterBuffer = gstConfig.SharedAudioRTPJitterBuffer
	mo.DefaultBitrate = config.SFU.Audio.DefaultBitrate
	mo.DefaultKBitrate = config.SFU.Audio.DefaultBitrate / 1000
}

func (mo *mediaOptions) addSharedVideoProperties() {
	// used in template or by template helpers
	mo.Rtp.JitterBuffer = gstConfig.SharedVideoRTPJitterBuffer
	mo.DefaultBitrate = config.SFU.Video.DefaultBitrate
	mo.DefaultKBitrate = config.SFU.Video.DefaultBitrate / 1000
}

// template helpers
func (mo mediaOptions) EncodeWith(name, nameSpace, filePrefix string) (output string) {
	output = strings.Replace(mo.Encoder, "{{.Name}}", name, -1)
	output = strings.Replace(output, "{{.Namespace}}", nameSpace, -1)
	output = strings.Replace(output, "{{.FilePrefix}}", filePrefix, -1)
	output = strings.Replace(output, "{{.DefaultBitrate}}", strconv.FormatUint(mo.DefaultBitrate, 10), -1)
	output = strings.Replace(output, "{{.DefaultKBitrate}}", strconv.FormatUint(mo.DefaultKBitrate, 10), -1)
	return
}

func (mo mediaOptions) ConstraintFormat() (output string) {
	output = strings.Replace(gstConfig.SharedVideoConstraintFormat, "{{.VideoFormat}}", gstConfig.SharedVideoFormat, -1)
	if mo.nvCuda {
		output = strings.Replace(output, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		output = strings.Replace(output, "{{.Convert}}", "videoconvert", -1)
	}
	return
}

func (mo mediaOptions) ConstraintFormatFramerate(framerate int) (output string) {
	caps := fmt.Sprintf("%v,framerate=%v/1", gstConfig.SharedVideoFormat, framerate)
	output = strings.Replace(gstConfig.SharedVideoConstraintFormatFramerateResolution, "{{.VideoFormatFramerateResolution}}", caps, -1)
	if mo.nvCuda {
		output = strings.Replace(output, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		output = strings.Replace(output, "{{.Convert}}", "videoconvert ! videoscale", -1)
	}
	return
}

func (mo mediaOptions) ConstraintFormatFramerateResolution(framerate, width, height int) (output string) {
	caps := fmt.Sprintf("%v,framerate=%v/1,width=%v,height=%v", gstConfig.SharedVideoFormat, framerate, width, height)
	output = strings.Replace(gstConfig.SharedVideoConstraintFormatFramerateResolution, "{{.VideoFormatFramerateResolution}}", caps, -1)
	if mo.nvCuda {
		output = strings.Replace(output, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		output = strings.Replace(output, "{{.Convert}}", "videoconvert ! videoscale", -1)
	}
	return
}

// func (mo mediaOptions) ConstraintFormatFramerateResolution(width, height, framerate int) (output string) {
// 	output = strings.Replace(gstConfig.SharedVideoConstraintFormatFramerateResolution, "{{.VideoCaps}}", gstConfig.SharedVideoFormat, -1)
// 	output = strings.Replace(output, "{{.Width}}", ", width="+strconv.Itoa(width), -1)
// 	output = strings.Replace(output, "{{.Height}}", ", height="+strconv.Itoa(height), -1)
// 	output = strings.Replace(output, "{{.Framerate}}", ", framerate="+strconv.Itoa(framerate)+"/1", -1)
// 	if mo.nvCuda {
// 		output = strings.Replace(output, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
// 	} else {
// 		output = strings.Replace(output, "{{.Convert}}", "videoconvert ! videoscale", -1)
// 	}
// 	return
// }
