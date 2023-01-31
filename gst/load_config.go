package gst

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type gstConfig struct {
	SharedAudioRTPJitterBuffer    string `yaml:"sharedAudioRTPJitterBuffer"`
	SharedVideoRTPJitterBuffer    string `yaml:"sharedVideoRTPJitterBuffer"`
	SharedVideoCapFormat          string `yaml:"sharedVideoCapFormat"`
	SharedVideoCapFormatRateScale string `yaml:"sharedVideoCapFormatRateScale"`
	Opus                          mediaOptions
	VP8                           mediaOptions `yaml:"vp8"`
	X264                          mediaOptions
	NV264                         mediaOptions `yaml:"nv264"`
}

// global state
var nvcodecEnv bool
var config gstConfig
var muxedRecordingTemplater, splitRecordingTemplater, passthroughTemplater, noRecordingTemplater *template.Template

func init() {
	nvcodecEnv = strings.ToLower(helpers.Getenv("DS_NVCODEC")) == "true"

	// load config from yml file
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

	// complete codec with shared properties
	config.Opus.addSharedAudioProperties()
	config.VP8.addSharedVideoProperties()
	config.X264.addSharedVideoProperties()
	config.NV264.addSharedVideoProperties()

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
