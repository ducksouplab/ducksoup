// Package gst provides an easy API to create an appsink pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"fmt"
	"sync"
	"time"
	"unsafe"

	"github.com/pion/webrtc/v3"
	"github.com/pion/webrtc/v3/pkg/media"
)

func init() {
	go C.gstreamer_send_start_mainloop()
}

// Pipeline is a wrapper for a GStreamer Pipeline
type Pipeline struct {
	Pipeline  *C.GstElement
	tracks    []*webrtc.TrackLocalStaticSample
	id        int
	codecName string
	clockRate float32
}

var pipelines = make(map[int]*Pipeline)
var pipelinesLock sync.Mutex

const (
	videoClockRate = 90000
	audioClockRate = 48000
	pcmClockRate   = 8000
)

// CreatePipeline creates a GStreamer Pipeline
func CreatePipeline(codecName string, tracks []*webrtc.TrackLocalStaticSample) *Pipeline {
	pipelineStr := "appsrc format=time is-live=true do-timestamp=true name=src ! application/x-rtp"
	// suffixPipelineStr := "autoaudiosink"
	suffixPipelineStr := "appsink name=appsink"
	// audioPipelineStr := "decodebin ! audioconvert ! audiochebband mode=band-pass lower-frequency=2000 upper-frequency=3000 poles=4 ! freeverb ! audioconvert"
	audioPipelineStr := "decodebin ! audioconvert ! pitch pitch=0.8 ! audioconvert"
	// videoPipelineStr := "decodebin ! videoconvert ! warptv ! videoconvert"
	videoPipelineStr := "decodebin ! videoconvert"
	var clockRate float32
	fmt.Println(codecName)

	switch codecName {
	case "vp8":
		// pipelineStr += ", media=video, clock-rate=90000, encoding-name=VP8-DRAFT-IETF-01, payload=100 ! rtpvp8depay ! vp8dec ! vp8enc error-resilient=partitions keyframe-max-dist=10 auto-alt-ref=true cpu-used=5 deadline=1 ! " + suffixPipelineStr
		pipelineStr += ", encoding-name=VP8-DRAFT-IETF-01 ! rtpvp8depay ! vp8dec ! vp8enc error-resilient=partitions keyframe-max-dist=10 auto-alt-ref=true cpu-used=5 deadline=1 ! " + suffixPipelineStr
		clockRate = videoClockRate
	case "vp9":
		pipelineStr += " ! rtpvp9depay ! " + videoPipelineStr + " ! vp9enc ! " + suffixPipelineStr
		clockRate = videoClockRate
	case "h264":
		pipelineStr += " ! rtph264depay ! " + videoPipelineStr + " ! video/x-raw,format=I420 ! x264enc speed-preset=ultrafast tune=zerolatency key-int-max=20 ! video/x-h264,stream-format=byte-stream ! " + suffixPipelineStr
		clockRate = videoClockRate
	case "G722":
		pipelineStr += " clock-rate=8000 ! rtpg722depay ! " + videoPipelineStr + " ! avenc_g722 ! " + suffixPipelineStr
		clockRate = audioClockRate
	case "opus":
		pipelineStr += ", payload=96, encoding-name=OPUS ! rtpopusdepay ! " + audioPipelineStr + " ! opusenc ! " + suffixPipelineStr
		clockRate = audioClockRate

	default:
		panic("Unhandled codec " + codecName)
	}

	pipelineStrUnsafe := C.CString(pipelineStr)
	defer C.free(unsafe.Pointer(pipelineStrUnsafe))

	pipelinesLock.Lock()
	defer pipelinesLock.Unlock()

	pipeline := &Pipeline{
		Pipeline:  C.gstreamer_send_create_pipeline(pipelineStrUnsafe),
		tracks:    tracks,
		id:        len(pipelines),
		codecName: codecName,
		clockRate: clockRate,
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
		for _, t := range pipeline.tracks {
			if err := t.WriteSample(media.Sample{Data: C.GoBytes(buffer, bufferLen), Duration: time.Duration(duration)}); err != nil {
				panic(err)
			}
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
