// Package gst provides an easy API to create an appsink pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v3"
)

type codecPipe struct {
	Prefix string `json:"prefix"`
	Suffix string `json:"suffix"`
}

type pluginConfig struct {
	SrcPrefix  string    `json:"srcPrefix"`
	SinkSuffix string    `json:"sinkSuffix"`
	AudioPipe  string    `json:"audioPipe"`
	VideoPipe  string    `json:"videoPipe"`
	Opus       codecPipe `json:"opus"`
	G722       codecPipe `json:"g722"`
	VP8        codecPipe `json:"vp8"`
	VP9        codecPipe `json:"vp9"`
	H264       codecPipe `json:"h264"`
}

var config pluginConfig

func init() {
	// config
	file, err := ioutil.ReadFile("./gst/config.json")
	if err != nil {
		fmt.Print(err)
	}
	err = json.Unmarshal(file, &config)
	if err != nil {
		fmt.Println("error:", err)
	}
}

func StartMainLoop() {
	C.gstreamer_send_start_mainloop()
}

// Pipeline is a wrapper for a GStreamer Pipeline
type Pipeline struct {
	Pipeline *C.GstElement
	track    *webrtc.TrackLocalStaticRTP
	id       int
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

func newPipelineStr(codecName string) (pipelineStr string) {
	codecName = strings.ToLower(codecName)
	pipelineStr = config.SrcPrefix
	isVideo := false

	switch codecName {
	case "vp8":
		pipelineStr += config.VP8.Prefix + config.VideoPipe + config.VP8.Suffix
		isVideo = true
	case "vp9":
		pipelineStr += config.VP9.Prefix + config.VideoPipe + config.VP9.Suffix
		isVideo = true
	case "h264":
		pipelineStr += config.H264.Prefix + config.VideoPipe + config.H264.Suffix
		isVideo = true
	case "g722":
		pipelineStr += config.G722.Prefix + config.AudioPipe + config.G722.Suffix
	case "opus":
		pipelineStr += config.Opus.Prefix + config.AudioPipe + config.Opus.Suffix
	default:
		panic("Unhandled codec " + codecName)
	}
	if isVideo {
		pipelineStr = fmt.Sprintf(pipelineStr, randomEffect())
	}
	pipelineStr += config.SinkSuffix
	return
}

// CreatePipeline creates a GStreamer Pipeline
func CreatePipeline(codecName string, track *webrtc.TrackLocalStaticRTP) *Pipeline {
	pipelineStr := newPipelineStr(codecName)
	fmt.Println(pipelineStr)

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()

	pipeline := &Pipeline{
		Pipeline: C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
		track:    track,
		id:       len(pipelines),
	}

	pipelines[pipeline.id] = pipeline
	return pipeline
}

// Start starts the GStreamer Pipeline
func (p *Pipeline) Start() {
	C.gstreamer_send_start_pipeline(p.Pipeline, C.int(p.id))
}

// Stop stops the GStreamer Pipeline
func (p *Pipeline) Stop() {
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
