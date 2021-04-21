// Package gst provides an easy API to create an appsink pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/creamlab/webrtc-transform/helpers"
	"github.com/pion/webrtc/v3"
)

var vp8Pipeline string
var opusPipeline string

func init() {
	vp8Pipeline = helpers.ReadConfig("vp8")
	opusPipeline = helpers.ReadConfig("opus")
}

func StartMainLoop() {
	C.gstreamer_send_start_mainloop()
}

// Pipeline is a wrapper for a GStreamer Pipeline
type Pipeline struct {
	Pipeline *C.GstElement
	track    *webrtc.TrackLocalStaticRTP
	id       int
	uid      string
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

func newPipelineStr(uid string, codecName string) (pipelineStr string) {
	codecName = strings.ToLower(codecName)

	switch codecName {
	case "vp8":
		pipelineStr = vp8Pipeline
		//pipelineStr = fmt.Sprintf(pipelineStr, randomEffect())
	case "opus":
		pipelineStr = opusPipeline
	default:
		panic("Unhandled codec " + codecName)
	}
	pipelineStr = strings.Replace(pipelineStr, "${uid}", uid, -1)
	return
}

// CreatePipeline creates a GStreamer Pipeline
func CreatePipeline(uid string, codecName string, track *webrtc.TrackLocalStaticRTP) *Pipeline {
	pipelineStr := newPipelineStr(uid, codecName)

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()

	pipeline := &Pipeline{
		Pipeline: C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
		track:    track,
		id:       len(pipelines),
		uid:      uid,
	}

	pipelines[pipeline.id] = pipeline
	return pipeline
}

// Start starts the GStreamer Pipeline
func (p *Pipeline) Start() {
	fmt.Printf("Pipeline started: %d %s\n", p.id, p.uid)
	C.gstreamer_send_start_pipeline(p.Pipeline, C.int(p.id))
}

// Stop stops the GStreamer Pipeline
func (p *Pipeline) Stop() {
	fmt.Printf("Pipeline stopped: %d %s\n", p.id, p.uid)
	C.gstreamer_send_stop_pipeline(p.Pipeline)
}

//export goHandlePipelineBuffer
func goHandlePipelineBuffer(buffer unsafe.Pointer, bufferLen C.int, duration C.int, pipelineID C.int) {
	pipelinesLock.Lock()
	pipeline, ok := pipelines[int(pipelineID)]
	pipelinesLock.Unlock()

	if ok {
		if _, err := pipeline.track.Write(C.GoBytes(buffer, bufferLen)); err != nil {
			panic(err)
		}
	} else {
		fmt.Printf("discarding buffer, no pipeline with id %d", int(pipelineID))
	}
	C.free(buffer)
}

// Push pushes a buffer on the appsrc of the GStreamer Pipeline
func (p *Pipeline) Push(buffer []byte) {
	b := C.CBytes(buffer)
	defer C.free(b)
	C.gstreamer_send_push_buffer(p.Pipeline, b, C.int(len(buffer)))
}
