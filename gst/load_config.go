package gst

import (
	"fmt"
	"text/template"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type gstEnhancedConfig struct {
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
var gstConfig gstEnhancedConfig
var muxedTemplater, splitTemplater, passthroughTemplater, noRecordingTemplater *template.Template

func init() {
	// load config from yml file
	f, err := helpers.Open("config/gst.yml")
	if err != nil {
		log.Fatal().Err(err)
	}
	defer f.Close()
	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&gstConfig)
	if err != nil {
		log.Fatal().Err(err)
	}

	// complete codec with shared properties
	gstConfig.Opus.addSharedAudioProperties()
	gstConfig.VP8.addSharedVideoProperties()
	gstConfig.X264.addSharedVideoProperties()
	gstConfig.NV264.addSharedVideoProperties()

	// templates
	muxedTemplater, err = template.New("muxed").Parse(helpers.ReadFile("config/pipelines/muxed.gtpl"))
	if err != nil {
		panic(err)
	}
	splitTemplater, err = template.New("splitRecording").Parse(helpers.ReadFile("config/pipelines/split.gtpl"))
	if err != nil {
		panic(err)
	}
	passthroughTemplater, err = template.New("passthrough").Parse(helpers.ReadFile("config/pipelines/passthrough.gtpl"))
	if err != nil {
		panic(err)
	}
	noRecordingTemplater, err = template.New("noRecording").Parse(helpers.ReadFile("config/pipelines/no_recording.gtpl"))
	if err != nil {
		panic(err)
	}

	// log
	log.Info().Str("context", "init").Str("config", fmt.Sprintf("%+v", gstConfig)).Msg("gstreamer_config_loaded")
}
