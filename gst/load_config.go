package gst

import (
	"text/template"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type gstreamerConfig struct {
	RTPJitterBuffer rtpJitterBuffer `yaml:"rtpJitterBuffer"`
	VP8             codec           `yaml:"vp8"`
	X264            codec
	NV264           codec `yaml:"nv264"`
	Opus            codec
}

type rtpJitterBuffer struct {
	Latency        string
	Retransmission string
}

type codec struct {
	Decode string
	Encode struct {
		Raw string
		Fx  string
	}
	Rtp struct {
		Caps  string
		Pay   string
		Depay string
	}
}

var pipelineTemplater *template.Template
var config gstreamerConfig

func init() {
	var err error
	pipelineTemplateString := helpers.ReadFile("config/pipeline.gtpl")
	pipelineTemplater, err = template.New("pipelineTemplate").Parse(pipelineTemplateString)
	if err != nil {
		panic(err)
	}

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

	// log
	log.Info().Msgf("[init] GStreamer config loaded: %+v", config)
}
