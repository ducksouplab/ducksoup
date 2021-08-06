package gst

import (
	"log"

	"github.com/creamlab/ducksoup/helpers"
	"gopkg.in/yaml.v2"
)

type Engine struct {
	Decode string
	Encode struct {
		Fast    string
		Relaxed string
	}
}

type Engines struct {
	VP8   Engine `yaml:"vp8"`
	X264  Engine
	NV264 Engine `yaml:"nv264"`
	Opus  Engine
}

var opusFxPipeline string
var opusRawPipeline string
var vp8FxPipeline string
var vp8RawPipeline string
var h264FxPipeline string
var h264RawPipeline string
var passthroughPipeline string
var engines Engines

func init() {
	opusFxPipeline = helpers.ReadFile("etc/opus-fx-rec.txt")
	opusRawPipeline = helpers.ReadFile("etc/opus-raw-rec.txt")
	vp8FxPipeline = helpers.ReadFile("etc/vp8-fx-rec.txt")
	vp8RawPipeline = helpers.ReadFile("etc/vp8-raw-rec.txt")
	h264FxPipeline = helpers.ReadFile("etc/h264-fx-rec.txt")
	h264RawPipeline = helpers.ReadFile("etc/h264-raw-rec.txt")
	passthroughPipeline = helpers.ReadFile("etc/passthrough.txt")

	f, err := helpers.Open("etc/engines.yml")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&engines)
	if err != nil {
		log.Fatal(err)
	}
}