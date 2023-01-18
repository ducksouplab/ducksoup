package gst

import (
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type gstreamerConfig struct {
	CommonAudioRTPJitterBuffer string `yaml:"commonAudioRTPJitterBuffer"`
	CommonVideoRTPJitterBuffer string `yaml:"commonVideoRTPJitterBuffer"`
	CommonAudioRawCaps         string `yaml:"commonAudioRawCaps"`
	CommonVideoRawCaps         string `yaml:"commonVideoRawCaps"`
	CommonVideoRawCapsLight    string `yaml:"commonVideoRawCapsLight"`
	Opus                       codec
	VP8                        codec `yaml:"vp8"`
	X264                       codec
	NV264                      codec `yaml:"nv264"`
}

type codec struct {
	Fx           string
	RawCaps      string // constraint width/height/framerate and more to ensure stability before muxer
	RawCapsLight string // don't constraint width/height/framerate, but only properties that a plugin might have changed
	Decode       string
	Encode       string
	Rtp          struct {
		Caps         string
		Pay          string
		Depay        string
		JitterBuffer string
	}
}

func (c codec) EncodeWith(name, nameSpace, filePrefix string) (output string) {
	output = strings.Replace(c.Encode, "{{.Name}}", name, -1)
	output = strings.Replace(output, "{{.Namespace}}", nameSpace, -1)
	output = strings.Replace(output, "{{.FilePrefix}}", filePrefix, -1)
	return
}

func (c codec) RawCapsWith(width, height, frameRate int) (output string) {
	output = strings.Replace(c.RawCaps, "{{.Width}}", ", width="+strconv.Itoa(width), -1)
	output = strings.Replace(output, "{{.Height}}", ", height="+strconv.Itoa(height), -1)
	output = strings.Replace(output, "{{.FrameRate}}", ", framerate="+strconv.Itoa(frameRate)+"/1", -1)
	return
}

var muxedRecordingTemplater, splitRecordingTemplater, passthroughTemplater, noRecordingTemplater *template.Template
var config gstreamerConfig

func init() {
	f, err := helpers.Open("config/gst.yml")
	if err != nil {
		log.Fatal().Err(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal().Err(err)
	}
	// complete codec with common properties
	config.Opus.Rtp.JitterBuffer = config.CommonAudioRTPJitterBuffer
	config.Opus.RawCaps = config.CommonAudioRawCaps
	config.VP8.Rtp.JitterBuffer = config.CommonVideoRTPJitterBuffer
	config.X264.Rtp.JitterBuffer = config.CommonVideoRTPJitterBuffer
	config.NV264.Rtp.JitterBuffer = config.CommonVideoRTPJitterBuffer
	config.VP8.RawCaps = config.CommonVideoRawCaps
	config.X264.RawCaps = config.CommonVideoRawCaps
	config.NV264.RawCaps = config.CommonVideoRawCaps
	config.VP8.RawCapsLight = config.CommonVideoRawCapsLight
	config.X264.RawCapsLight = config.CommonVideoRawCapsLight
	config.NV264.RawCapsLight = config.CommonVideoRawCapsLight

	// templates
	muxedRecordingTemplater, err = template.New("muxedRecording").Parse(helpers.ReadFile("config/pipelines/muxed_recording.gtpl"))
	if err != nil {
		panic(err)
	}
	splitRecordingTemplater, err = template.New("splitRecording").Parse(helpers.ReadFile("config/pipelines/split_recording.gtpl"))
	if err != nil {
		panic(err)
	}
	passthroughTemplater, err = template.New("passthrough").Parse(helpers.ReadFile("config/pipelines/split_recording_passthrough.gtpl"))
	if err != nil {
		panic(err)
	}
	noRecordingTemplater, err = template.New("noRecording").Parse(helpers.ReadFile("config/pipelines/no_recording.gtpl"))
	if err != nil {
		panic(err)
	}

	// log
	log.Info().Str("context", "init").Str("config", fmt.Sprintf("%+v", config)).Msg("gstreamer_config_loaded")
}
