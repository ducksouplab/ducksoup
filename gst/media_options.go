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
	DefaultBitrate  int
	DefaultKBitrate int
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
	TimeOverlay string
}

func (mo *mediaOptions) addSharedAudioProperties() {
	// used in template or by template helpers
	mo.Rtp.JitterBuffer = gstConfig.Shared.Audio.RTPJitterBuffer
	mo.DefaultBitrate = config.SFU.Audio.DefaultBitrate
	mo.DefaultKBitrate = config.SFU.Audio.DefaultBitrate / 1000
}

func (mo *mediaOptions) addSharedVideoProperties() {
	// used in template or by template helpers
	mo.Rtp.JitterBuffer = gstConfig.Shared.Video.RTPJitterBuffer
	mo.DefaultBitrate = config.SFU.Video.DefaultBitrate
	mo.DefaultKBitrate = config.SFU.Video.DefaultBitrate / 1000
	mo.TimeOverlay = gstConfig.Shared.Video.TimeOverlay
}

// template helpers

func (mo mediaOptions) EncodeWith(name string) (output string) {
	output = strings.Replace(mo.Encoder, "{{.Name}}", name, -1)
	output = strings.Replace(output, "{{.DefaultBitrate}}", strconv.Itoa(mo.DefaultBitrate), -1)
	output = strings.Replace(output, "{{.DefaultKBitrate}}", strconv.Itoa(mo.DefaultKBitrate), -1)
	return
}

func (mo mediaOptions) EncodeWithCache(name, folder, filePrefix string) (output string) {
	output = mo.EncodeWith(name)
	output = strings.Replace(output, "{{.Folder}}", folder, -1)
	output = strings.Replace(output, "{{.FilePrefix}}", filePrefix, -1)
	return
}

func (mo mediaOptions) ConstraintFormat() (output string) {
	output = strings.Replace(gstConfig.Shared.Video.Constraint.Format, "{{.VideoFormat}}", gstConfig.Shared.Video.RawFormat, -1)
	if mo.nvCuda {
		output = strings.Replace(output, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		output = strings.Replace(output, "{{.Convert}}", "videoconvert", -1)
	}
	return
}

func (mo mediaOptions) ConstraintFormatFramerate(framerate int) (output string) {
	caps := fmt.Sprintf("%v,framerate=%v/1", gstConfig.Shared.Video.RawFormat, framerate)
	output = strings.Replace(gstConfig.Shared.Video.Constraint.FormatFramerateResolution, "{{.VideoFormatFramerateResolution}}", caps, -1)
	if mo.nvCuda {
		output = strings.Replace(output, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		output = strings.Replace(output, "{{.Convert}}", "videoconvert ! videoscale", -1)
	}
	return
}

func (mo mediaOptions) ConstraintFormatFramerateResolution(framerate, width, height int) (output string) {
	caps := fmt.Sprintf("%v,framerate=%v/1,width=%v,height=%v", gstConfig.Shared.Video.RawFormat, framerate, width, height)
	output = strings.Replace(gstConfig.Shared.Video.Constraint.FormatFramerateResolution, "{{.VideoFormatFramerateResolution}}", caps, -1)
	if mo.nvCuda {
		output = strings.Replace(output, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		output = strings.Replace(output, "{{.Convert}}", "videoconvert ! videoscale", -1)
	}
	return
}
