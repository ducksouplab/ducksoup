// Package gst provides an easy API to create an appsink pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"log"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/pion/webrtc/v3"
)

var opusFxPipeline string
var opusRawPipeline string
var vp8FxPipeline string
var vp8RawPipeline string
var h264FxPipeline string
var h264RawPipeline string
var passthroughPipeline string

const (
	DefaultVP8Enc = "vp8enc deadline=1 cpu-used=4 end-usage=1 target-bitrate=300000 undershoot=95 keyframe-max-dist=999999 max-quantizer=56 qos=true"
	// Previous: vp8enc keyframe-max-dist=64 resize-allowed=true dropframe-threshold=25 max-quantizer=56 cpu-used=5 threads=4 deadline=1 qos=true
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

// Pipeline is a wrapper for a GStreamer pipeline and output track
type Pipeline struct {
	Pipeline   *C.GstElement
	Files      []string
	track      *webrtc.TrackLocalStaticRTP
	id         int
	filePrefix string
}

var pipelines = make(map[int]*Pipeline)
var pipelinesLock sync.Mutex

func newPipelineStr(filePrefix string, kind string, codecName string, width int, height int, frameRate int, fx string) (pipelineStr string) {

	// special case for testing
	if fx == "passthrough" {
		pipelineStr = passthroughPipeline
		return
	}

	hasFx := len(fx) > 0

	switch codecName {
	case "opus":
		if hasFx {
			pipelineStr = opusFxPipeline
		} else {
			pipelineStr = opusRawPipeline
		}
	case "VP8":
		if hasFx {
			pipelineStr = vp8FxPipeline
		} else {
			pipelineStr = vp8RawPipeline
		}
		pipelineStr = strings.Replace(pipelineStr, "${encode}", DefaultVP8Enc, -1)
	case "H264":
		if hasFx {
			pipelineStr = h264FxPipeline
		} else {
			pipelineStr = h264RawPipeline
		}
	default:
		panic("Unhandled codec " + codecName)
	}
	// set file
	pipelineStr = strings.Replace(pipelineStr, "${prefix}", filePrefix, -1)
	// set fx
	if hasFx {
		// add "fx" prefix to avoid name clashes (for instance if a user gives the name "src")
		prefixedFx := strings.Replace(fx, "name=", "name=fx", 1)
		pipelineStr = strings.Replace(pipelineStr, "${fx}", prefixedFx, -1)
	}
	// set dimensionts
	pipelineStr = strings.Replace(pipelineStr, "${width}", strconv.Itoa(width), -1)
	pipelineStr = strings.Replace(pipelineStr, "${height}", strconv.Itoa(height), -1)
	pipelineStr = strings.Replace(pipelineStr, "${framerate}", strconv.Itoa(frameRate), -1)
	return
}

func fileName(prefix string, kind string, suffix string) string {
	ext := ".mkv"
	if kind == "audio" {
		ext = ".ogg"
	}
	return prefix + "-" + kind + "-" + suffix + ext
}

func allFiles(prefix string, kind string, hasFx bool) []string {
	if hasFx {
		return []string{fileName(prefix, kind, "raw"), fileName(prefix, kind, "fx")}
	} else {
		return []string{fileName(prefix, kind, "raw")}
	}
}

//export goHandleNewSample
func goHandleNewSample(pipelineId C.int, buffer unsafe.Pointer, bufferLen C.int, duration C.int) {
	pipelinesLock.Lock()
	pipeline, ok := pipelines[int(pipelineId)]
	pipelinesLock.Unlock()

	if ok {
		if _, err := pipeline.track.Write(C.GoBytes(buffer, bufferLen)); err != nil {
			// TODO err contains the ID of the failing PeerConnections
			// we may store a callback on the Pipeline struct (the callback would remove those peers and update signaling)
			log.Printf("[gst] error: %v", err)
		}
	} else {
		// TODO return error to gst.c and stop processing?
		log.Printf("[gst] discarding buffer, no pipeline with id %d", int(pipelineId))
	}
	C.free(buffer)
}

// API

func StartMainLoop() {
	C.gstreamer_start_mainloop()
}

// create a GStreamer pipeline
func CreatePipeline(track *webrtc.TrackLocalStaticRTP, filePrefix string, kind string, codecName string, width int, height int, frameRate int, fx string) *Pipeline {

	pipelineStr := newPipelineStr(filePrefix, kind, codecName, width, height, frameRate, fx)
	log.Printf("[gst] %v pipeline: %v", kind, pipelineStr)

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()

	pipeline := &Pipeline{
		Pipeline:   C.gstreamer_parse_pipeline(pipelineStrUnsafe),
		Files:      allFiles(filePrefix, kind, len(fx) > 0),
		track:      track,
		id:         len(pipelines),
		filePrefix: filePrefix,
	}

	pipelines[pipeline.id] = pipeline
	return pipeline
}

// start the GStreamer pipeline
func (p *Pipeline) Start() {
	C.gstreamer_start_pipeline(p.Pipeline, C.int(p.id))
	log.Printf("[gst] pipeline %d started: %s\n", p.id, p.filePrefix)
}

// stop the GStreamer pipeline
func (p *Pipeline) Stop() {
	C.gstreamer_stop_pipeline(p.Pipeline, C.int(p.id))
	log.Printf("[gst] pipeline %d stopped: %s\n", p.id, p.filePrefix)
}

// push a buffer on the appsrc of the GStreamer Pipeline
func (p *Pipeline) Push(buffer []byte) {
	b := C.CBytes(buffer)
	defer C.free(b)
	C.gstreamer_push_buffer(p.Pipeline, b, C.int(len(buffer)))
}

func (p *Pipeline) SetFxProperty(elName string, elProperty string, elValue float32) {
	// fx prefix needed (added during pipeline initialization)
	cName := C.CString("fx" + elName)
	cProperty := C.CString(elProperty)
	cValue := C.float(elValue)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProperty))

	C.gstreamer_set_fx_property(p.Pipeline, cName, cProperty, cValue)
}

func (p *Pipeline) GetFxProperty(elName string, elProperty string) float32 {
	// fx prefix needed (added during pipeline initialization)
	cName := C.CString("fx" + elName)
	cProperty := C.CString(elProperty)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProperty))

	return float32(C.gstreamer_get_fx_property(p.Pipeline, cName, cProperty))
}
