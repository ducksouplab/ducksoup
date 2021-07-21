package gst

import (
	"github.com/creamlab/ducksoup/helpers"
)

var opusFxPipeline string
var opusRawPipeline string
var vp8FxPipeline string
var vp8RawPipeline string
var h264FxPipeline string
var h264RawPipeline string
var passthroughPipeline string

const (
	// Previous: vp8enc keyframe-max-dist=64 resize-allowed=true dropframe-threshold=25 max-quantizer=56 cpu-used=5 threads=4 deadline=1 qos=true
	vp8EncRT  = "vp8enc deadline=1 cpu-used=4 end-usage=1 target-bitrate=300000 undershoot=95 keyframe-max-dist=999999 max-quantizer=56 qos=true"
	vp8Enc    = "vp8enc deadline=1 cpu-used=4 end-usage=1 target-bitrate=300000 undershoot=95 keyframe-max-dist=999999 max-quantizer=56"
	h264EncRT = "x264enc pass=5 quantizer=15 speed-preset=superfast key-int-max=64 tune=zerolatency qos=true ! video/x-h264,stream-format=byte-stream,profile=main"
	h264Enc   = "x264enc pass=5 quantizer=15 speed-preset=superfast key-int-max=64 ! video/x-h264,stream-format=byte-stream,profile=main"
)

func init() {
	opusFxPipeline = helpers.ReadTextFile("config/gst/opus-fx-rec.txt")
	opusRawPipeline = helpers.ReadTextFile("config/gst/opus-raw-rec.txt")
	vp8FxPipeline = helpers.ReadTextFile("config/gst/vp8-fx-rec.txt")
	vp8RawPipeline = helpers.ReadTextFile("config/gst/vp8-raw-rec.txt")
	h264FxPipeline = helpers.ReadTextFile("config/gst/h264-fx-rec.txt")
	h264RawPipeline = helpers.ReadTextFile("config/gst/h264-raw-rec.txt")
	passthroughPipeline = helpers.ReadTextFile("config/gst/passthrough.txt")
}
