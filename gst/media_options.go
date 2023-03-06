package gst

import (
	"strconv"
	"strings"
)

// capitalized props are accessible to template
type mediaOptions struct {
	// calculated property
	SkipFixedCaps bool
	// live property depending on Join (and DUCKSOUP_NVCODEC), not used within template
	nvcodec bool
	Overlay bool
	// properties depending on yml definitions
	Fx      string
	Decoder string
	Encoder string
	Cap     struct {
		Format          string // don't constraint width/height/framerate, but only properties that a plugin might have changed
		FormatRateScale string // constraint width/height/framerate and more to ensure stability before muxer
	}
	Rtp struct {
		Caps         string
		Pay          string
		Depay        string
		JitterBuffer string
	}
}

func (mo *mediaOptions) addSharedAudioProperties() {
	mo.Rtp.JitterBuffer = config.SharedAudioRTPJitterBuffer
}

func (mo *mediaOptions) addSharedVideoProperties() {
	mo.Rtp.JitterBuffer = config.SharedVideoRTPJitterBuffer
	mo.Cap.Format = config.SharedVideoCapFormat
	mo.Cap.FormatRateScale = config.SharedVideoCapFormatRateScale
}

// template helpers
func (mo mediaOptions) EncodeWith(name, nameSpace, filePrefix string) (output string) {
	output = strings.Replace(mo.Encoder, "{{.Name}}", name, -1)
	output = strings.Replace(output, "{{.Namespace}}", nameSpace, -1)
	output = strings.Replace(output, "{{.FilePrefix}}", filePrefix, -1)
	return
}

func (mo mediaOptions) CapFormatOnly() string {
	if mo.nvcodec {
		return strings.Replace(mo.Cap.Format, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		return strings.Replace(mo.Cap.Format, "{{.Convert}}", "videoconvert", -1)
	}
}

func (mo mediaOptions) CapFormatRateScale(width, height, frameRate int) (output string) {
	if mo.nvcodec {
		output = strings.Replace(mo.Cap.FormatRateScale, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		output = strings.Replace(mo.Cap.FormatRateScale, "{{.Convert}}", "videoconvert ! videoscale", -1)
	}
	output = strings.Replace(output, "{{.Width}}", ", width="+strconv.Itoa(width), -1)
	output = strings.Replace(output, "{{.Height}}", ", height="+strconv.Itoa(height), -1)
	output = strings.Replace(output, "{{.FrameRate}}", ", framerate="+strconv.Itoa(frameRate)+"/1", -1)
	return
}
