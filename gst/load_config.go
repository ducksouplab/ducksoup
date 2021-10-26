package gst

import (
	"log"

	"github.com/creamlab/ducksoup/helpers"
	"gopkg.in/yaml.v2"
)

type gstreamerConfig struct {
	ForceEncodingSize bool `yaml:"forceEncodingSize"`
	RTPJitterBuffer   struct {
		Latency        string
		Retransmission string
	} `yaml:"rtpJitterBuffer"`
	VP8   codec `yaml:"vp8"`
	X264  codec
	NV264 codec `yaml:"nv264"`
	Opus  codec
}

type codec struct {
	Decode string
	Encode struct {
		Fast    string
		Relaxed string
	}
}

var opusFxPipeline string
var opusRawPipeline string
var vp8FxPipeline string
var vp8RawPipeline string
var h264FxPipeline string
var h264RawPipeline string
var passthroughPipeline string
var config gstreamerConfig

func init() {
	opusFxPipeline = helpers.ReadFile("config/pipelines/opus-fx-rec.txt")
	opusRawPipeline = helpers.ReadFile("config/pipelines/opus-nofx-rec.txt")
	vp8FxPipeline = helpers.ReadFile("config/pipelines/vp8-fx-rec.txt")
	vp8RawPipeline = helpers.ReadFile("config/pipelines/vp8-nofx-rec.txt")
	h264FxPipeline = helpers.ReadFile("config/pipelines/h264-fx-rec.txt")
	h264RawPipeline = helpers.ReadFile("config/pipelines/h264-nofx-rec.txt")
	passthroughPipeline = helpers.ReadFile("config/pipelines/passthrough.txt")

	f, err := helpers.Open("config/gst.yml")
	if err != nil {
		log.Fatal("[fatal] ", err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal("[fatal] ", err)
	}

	// log
	log.SetFlags(log.Lmicroseconds)
	log.Printf("[info] [init] gstreamer config: %+v\n", config)
}
