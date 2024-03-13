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
		Video struct {
			RawFormat  string `yaml:"rawFormat"`
			Constraint struct {
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

var templateNames = []string{"audio_only_no_recording", "audio_only", "direct", "muxed_forced_framerate", "muxed_free_framerate", "muxed_reenc_dry", "no_recording", "rtpbin_only", "split"}

// global state
var gstConfig gstEnhancedConfig

var templateIndex map[string]*template.Template

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
	templateIndex = make(map[string]*template.Template)
	for _, name := range templateNames {
		t, err := template.New("name").Parse(helpers.ReadFile("config/pipelines/" + name + ".gtpl"))
		if err != nil {
			panic(err)
		}
		templateIndex[name] = t
	}

	// log
	log.Info().Str("context", "init").Str("config", fmt.Sprintf("%+v", gstConfig)).Msg("gstreamer_config_loaded")
}
