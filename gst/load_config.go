package gst

import (
	"fmt"
	"text/template"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type queueConfig struct {
	Base  string
	Short string
	Leaky string
	Long  string
}

type gstEnhancedConfig struct {
	Shared struct {
		Audio struct {
			RTPJitterBuffer string `yaml:"RTPJitterBuffer"`
		}
		Video struct {
			RTPJitterBuffer string `yaml:"RTPJitterBuffer"`
			RawFormat       string `yaml:"rawFormat"`
			Constraint      struct {
				Format                    string
				FormatFramerate           string `yaml:"formatFramerate"`
				FormatFramerateResolution string `yaml:"formatFramerateResolution"`
			}
			TimeOverlay string `yaml:"timeOverlay"`
		}
		Queue queueConfig
	}
	Opus  mediaOptions
	VP8   mediaOptions `yaml:"vp8"`
	X264  mediaOptions
	NV264 mediaOptions `yaml:"nv264"`
}

// global state
var gstConfig gstEnhancedConfig
var muxedTemplater, muxedReencTemplater, splitTemplater, passthroughTemplater, noRecordingTemplater, audioOnlyTemplater, audioOnlyPassthroughTemplater, audioOnlyNoRecordingTemplater *template.Template

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
	muxedReencTemplater, err = template.New("muxed_reenc").Parse(helpers.ReadFile("config/pipelines/muxed_reenc.gtpl"))
	if err != nil {
		panic(err)
	}
	splitTemplater, err = template.New("split").Parse(helpers.ReadFile("config/pipelines/split.gtpl"))
	if err != nil {
		panic(err)
	}
	passthroughTemplater, err = template.New("passthrough").Parse(helpers.ReadFile("config/pipelines/passthrough.gtpl"))
	if err != nil {
		panic(err)
	}
	noRecordingTemplater, err = template.New("no_recording").Parse(helpers.ReadFile("config/pipelines/no_recording.gtpl"))
	if err != nil {
		panic(err)
	}
	audioOnlyTemplater, err = template.New("audio_only").Parse(helpers.ReadFile("config/pipelines/audio_only.gtpl"))
	if err != nil {
		panic(err)
	}
	audioOnlyPassthroughTemplater, err = template.New("audio_only_passthrough").Parse(helpers.ReadFile("config/pipelines/audio_only_passthrough.gtpl"))
	if err != nil {
		panic(err)
	}
	audioOnlyNoRecordingTemplater, err = template.New("audio_only_no_recording").Parse(helpers.ReadFile("config/pipelines/audio_only_no_recording.gtpl"))
	if err != nil {
		panic(err)
	}

	// log
	log.Info().Str("context", "init").Str("config", fmt.Sprintf("%+v", gstConfig)).Msg("gstreamer_config_loaded")
}
