package gst

import (
	"text/template"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type gstreamerConfig struct {
	MuxRecords      bool            `yaml:"muxRecords"`
	RTPJitterBuffer rtpJitterBuffer `yaml:"rtpJitterBuffer"`
	VideoFormat     string          `yaml:"videoFormat"`
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

	// rely on decoded config
	pipelineTemplateFile := "config/pipeline_muxed.gtpl"
	if !config.MuxRecords {
		pipelineTemplateFile = "config/pipeline_split.gtpl"
	}
	pipelineTemplateString := helpers.ReadFile(pipelineTemplateFile)
	pipelineTemplater, err = template.New("pipelineTemplate").Parse(pipelineTemplateString)
	if err != nil {
		panic(err)
	}

	// log
	log.Info().Msgf("[init] GStreamer config loaded: %+v", config)
}
