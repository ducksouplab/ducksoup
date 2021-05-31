// Package gst provides an easy API to create an appsink pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/pion/webrtc/v3"
)

var opusProcPipeline string
var opusRawPipeline string
var vp8ProcPipeline string
var vp8RawPipeline string
var h264ProcPipeline string
var h264RawPipeline string

func init() {
	opusProcPipeline = helpers.ReadConfig("opus-proc-rec")
	opusRawPipeline = helpers.ReadConfig("opus-raw-rec")
	vp8ProcPipeline = helpers.ReadConfig("vp8-proc-rec")
	vp8RawPipeline = helpers.ReadConfig("vp8-raw-rec")
	h264ProcPipeline = helpers.ReadConfig("h264-norec")
	h264RawPipeline = helpers.ReadConfig("h264-norec")
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

func randomEffect() string {
	rand.Seed(time.Now().Unix())
	// options := []string{
	// 	"rippletv", "dicetv", "edgetv", "optv", "quarktv", "radioactv", "warptv", "shagadelictv", "streaktv", "vertigotv",
	// }
	options := []string{"identity"}
	return options[rand.Intn(len(options))]
}

func newPipelineStr(filePrefix string, codecName string, proc bool) (pipelineStr string) {
	codecName = strings.ToLower(codecName)

	switch codecName {
	case "opus":
		if proc {
			pipelineStr = opusProcPipeline
		} else {
			pipelineStr = opusRawPipeline
		}
	case "vp8":
		if proc {
			pipelineStr = vp8ProcPipeline
		} else {
			pipelineStr = vp8RawPipeline
		}
	case "h264":
		if proc {
			pipelineStr = h264ProcPipeline
		} else {
			pipelineStr = h264RawPipeline
		}
	default:
		panic("Unhandled codec " + codecName)
	}
	pipelineStr = strings.Replace(pipelineStr, "${prefix}", filePrefix, -1)
	return
}

func fileName(prefix string, kind string, suffix string) string {
	return prefix + "-" + kind + "-" + suffix
}

func allFiles(prefix string, kind string, proc bool) []string {
	if proc {
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
func CreatePipeline(track *webrtc.TrackLocalStaticRTP, filePrefix string, kind string, codecName string, proc bool) *Pipeline {
	pipelineStr := newPipelineStr(filePrefix, codecName, proc)

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()

	pipeline := &Pipeline{
		Pipeline:   C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
		Files:      allFiles(filePrefix, kind, proc),
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
