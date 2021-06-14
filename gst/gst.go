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

const (
	DefaultWidth   = 800
	DefaultHeight  = 600
	DefaultAudioFx = "pitch pitch=0.8"
	DefaultVideoFx = "coloreffects preset=xpro"
)

func init() {
	opusFxPipeline = helpers.ReadConfig("opus-fx-rec")
	opusRawPipeline = helpers.ReadConfig("opus-raw-rec")
	vp8FxPipeline = helpers.ReadConfig("vp8-fx-rec")
	vp8RawPipeline = helpers.ReadConfig("vp8-raw-rec")
	h264FxPipeline = helpers.ReadConfig("h264-fx-rec")
	h264RawPipeline = helpers.ReadConfig("h264-raw-rec")
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

	codecName = strings.ToLower(codecName)
	hasFx := len(fx) > 0

	switch codecName {
	case "opus":
		if hasFx {
			pipelineStr = opusFxPipeline
		} else {
			pipelineStr = opusRawPipeline
		}
	case "vp8":
		if hasFx {
			pipelineStr = vp8FxPipeline
		} else {
			pipelineStr = vp8RawPipeline
		}
	case "h264":
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
		pipelineStr = strings.Replace(pipelineStr, "${fx}", fx, -1)
	}
	// set dimensionts
	pipelineStr = strings.Replace(pipelineStr, "${width}", strconv.Itoa(width), -1)
	pipelineStr = strings.Replace(pipelineStr, "${height}", strconv.Itoa(height), -1)
	pipelineStr = strings.Replace(pipelineStr, "${framerate}", strconv.Itoa(frameRate), -1)
	log.Printf("[gst] %v pipeline: %v", kind, pipelineStr)
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
		return []string{fileName(prefix, kind, "in"), fileName(prefix, kind, "out")}
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
	C.gstreamer_send_start_mainloop()
}

// create a GStreamer pipeline
func CreatePipeline(track *webrtc.TrackLocalStaticRTP, filePrefix string, kind string, codecName string, width int, height int, frameRate int, fx string) *Pipeline {

	pipelineStr := newPipelineStr(filePrefix, kind, codecName, width, height, frameRate, fx)

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()

	pipeline := &Pipeline{
		Pipeline:   C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
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
	log.Printf("[gst] pipeline started: %d %s\n", p.id, p.filePrefix)
	C.gstreamer_send_start_pipeline(p.Pipeline, C.int(p.id))
}

// stop the GStreamer pipeline
func (p *Pipeline) Stop() {
	log.Printf("[gst] pipeline stopped: %d %s\n", p.id, p.filePrefix)
	C.gstreamer_send_stop_pipeline(p.Pipeline)
}

// push a buffer on the appsrc of the GStreamer Pipeline
func (p *Pipeline) Push(buffer []byte) {
	b := C.CBytes(buffer)
	defer C.free(b)
	C.gstreamer_send_push_buffer(p.Pipeline, b, C.int(len(buffer)))
}
