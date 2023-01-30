package gst

import (
	"strconv"
	"strings"
)

type codec struct {
	// live property depending on Join
	GPU bool
	// properties depending on yml definitions
	Fx      string
	Decode  string
	Encode  string
	Convert struct {
		Color          string // don't constraint width/height/framerate, but only properties that a plugin might have changed
		ColorRateScale string // constraint width/height/framerate and more to ensure stability before muxer
	}
	Rtp struct {
		Caps         string
		Pay          string
		Depay        string
		JitterBuffer string
	}
}

func (c *codec) addSharedAudioProperties() {
	c.Rtp.JitterBuffer = config.SharedAudioRTPJitterBuffer
}

func (c *codec) addSharedVideoProperties() {
	c.Rtp.JitterBuffer = config.SharedVideoRTPJitterBuffer
	c.Convert.Color = config.SharedVideoConvertColor
	c.Convert.ColorRateScale = config.SharedVideoConvertColorRateScale
}

// template helpers
func (c codec) EncodeWith(name, nameSpace, filePrefix string) (output string) {
	output = strings.Replace(c.Encode, "{{.Name}}", name, -1)
	output = strings.Replace(output, "{{.Namespace}}", nameSpace, -1)
	output = strings.Replace(output, "{{.FilePrefix}}", filePrefix, -1)
	return
}

func (c codec) ConvertColorOnly() string {
	if nvidiaEnabled && c.GPU {
		return strings.Replace(c.Convert.Color, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		return strings.Replace(c.Convert.Color, "{{.Convert}}", "videoconvert", -1)
	}
}

func (c codec) ConvertColorRateScale(width, height, frameRate int) (output string) {
	if nvidiaEnabled && c.GPU {
		output = strings.Replace(c.Convert.ColorRateScale, "{{.Convert}}", "cudaupload ! cudaconvertscale ! cudadownload", -1)
	} else {
		output = strings.Replace(c.Convert.ColorRateScale, "{{.Convert}}", "videoconvert ! videoscale", -1)
	}
	output = strings.Replace(output, "{{.Width}}", ", width="+strconv.Itoa(width), -1)
	output = strings.Replace(output, "{{.Height}}", ", height="+strconv.Itoa(height), -1)
	output = strings.Replace(output, "{{.FrameRate}}", ", framerate="+strconv.Itoa(frameRate)+"/1", -1)
	return
}
