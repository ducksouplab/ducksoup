package gst

import (
	"log"

	"github.com/creamlab/ducksoup/helpers"
	"gopkg.in/yaml.v2"
)

type CommonSettings struct {
	JitterBufferLatency string `yaml:"jitterBufferLatency"`
}

type EngineSettings struct {
	Decode string
	Encode struct {
		Fast    string
		Relaxed string
	}
}

type Settings struct {
	Common CommonSettings
	VP8    EngineSettings `yaml:"vp8"`
	X264   EngineSettings
	NV264  EngineSettings `yaml:"nv264"`
	Opus   EngineSettings
}

var opusFxPipeline string
var opusRawPipeline string
var vp8FxPipeline string
var vp8RawPipeline string
var h264FxPipeline string
var h264RawPipeline string
var passthroughPipeline string
var settings Settings

func init() {
	opusFxPipeline = helpers.ReadFile("etc/opus-fx-rec.txt")
	opusRawPipeline = helpers.ReadFile("etc/opus-nofx-rec.txt")
	vp8FxPipeline = helpers.ReadFile("etc/vp8-fx-rec.txt")
	vp8RawPipeline = helpers.ReadFile("etc/vp8-nofx-rec.txt")
	h264FxPipeline = helpers.ReadFile("etc/h264-fx-rec.txt")
	h264RawPipeline = helpers.ReadFile("etc/h264-nofx-rec.txt")
	passthroughPipeline = helpers.ReadFile("etc/passthrough.txt")

	f, err := helpers.Open("etc/settings.yml")
	if err != nil {
		log.Fatal("[fatal] ", err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&settings)
	if err != nil {
		log.Fatal("[fatal] ", err)
	}
}
