// Package gst provides an easy API to create a GStreamer pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/ducksouplab/ducksoup/types"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// global state
var (
	nvidiaEnabled bool
)

func init() {
	nvidiaEnabled = strings.ToLower(helpers.Getenv("DS_NVIDIA")) == "true"
}

// Pipeline is a wrapper for a GStreamer pipeline and output track
type Pipeline struct {
	mu          sync.Mutex
	id          string // same as local/output track id
	join        types.JoinPayload
	cPipeline   *C.GstElement
	audioOutput types.TrackWriter
	videoOutput types.TrackWriter
	filePrefix  string
	// stoppedCount=2 if audio and video have been stopped
	stoppedCount int
	// log
	logger zerolog.Logger
}

func fileName(namespace string, prefix string, suffix string) string {
	return namespace + "/" + prefix + "-" + suffix + ".mkv"
}

// API

func StartMainLoop() {
	C.gstStartMainLoop()
}

// create a GStreamer pipeline
func NewPipeline(join types.JoinPayload, filePrefix string) *Pipeline {

	pipelineStr := newPipelineDef(join, filePrefix)
	id := uuid.New().String()

	cPipelineStr := C.CString(pipelineStr)
	cId := C.CString(id)
	defer C.free(unsafe.Pointer(cPipelineStr))
	defer C.free(unsafe.Pointer(cId))

	logger := log.With().
		Str("context", "pipeline").
		Str("namespace", join.Namespace).
		Str("room", join.RoomId).
		Str("user", join.UserId).
		Str("pipeline", id).
		Logger()

	p := &Pipeline{
		mu:           sync.Mutex{},
		id:           id,
		join:         join,
		cPipeline:    C.gstParsePipeline(cPipelineStr, cId),
		stoppedCount: 0,
		logger:       logger,
	}

	p.logger.Info().Str("pipeline", pipelineStr).Msg("pipeline_initialized")

	pipelineStoreSingleton.add(p)
	return p
}

func (p *Pipeline) OutputFiles() []string {
	namespace := p.join.Namespace
	hasFx := len(p.join.AudioFx) > 0 || len(p.join.VideoFx) > 0
	if hasFx {
		return []string{fileName(namespace, p.filePrefix, "dry"), fileName(namespace, p.filePrefix, "wet")}
	} else {
		return []string{fileName(namespace, p.filePrefix, "dry")}
	}
}

func (p *Pipeline) PushRTP(kind string, buffer []byte) {
	s := C.CString(kind + "_src")
	defer C.free(unsafe.Pointer(s))

	b := C.CBytes(buffer)
	defer C.free(b)
	C.gstPushBuffer(s, p.cPipeline, b, C.int(len(buffer)))
}

func (p *Pipeline) PushRTCP(kind string, buffer []byte) {
	s := C.CString(kind + "_buffer")
	defer C.free(unsafe.Pointer(s))

	b := C.CBytes(buffer)
	defer C.free(b)
	//C.gstPushRTCPBuffer(s, p.cPipeline, b, C.int(len(buffer)))
}

func (p *Pipeline) BindTrack(kind string, t types.TrackWriter) {
	if kind == "audio" {
		p.audioOutput = t
	} else {
		p.videoOutput = t
	}
}

func (p *Pipeline) IsReady() bool {
	return p.audioOutput != nil && p.videoOutput != nil
}

func (p *Pipeline) Start() {
	// GStreamer start
	C.gstStartPipeline(p.cPipeline)
	recording_prefix := fmt.Sprintf("%s/%s", p.join.Namespace, p.filePrefix)
	p.logger.Info().Str("recording_prefix", recording_prefix).Msg("pipeline_started")
}

// stop the GStreamer pipeline
func (p *Pipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stoppedCount += 1
	if p.stoppedCount == 2 { // audio and video buffers from mixerSlice have been stopped
		C.gstStopPipeline(p.cPipeline)
		p.logger.Info().Msg("pipeline_stopped")
	}
}

func (p *Pipeline) getPropInt(name string, prop string) int {
	cName := C.CString(name)
	cProp := C.CString(prop)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	return int(C.gstGetPropInt(p.cPipeline, cName, cProp))
}

func (p *Pipeline) setPropInt(name string, prop string, value int) {
	// fx prefix needed (added during pipeline initialization)
	cName := C.CString(name)
	cProp := C.CString(prop)
	cValue := C.int(value)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	C.gstSetPropInt(p.cPipeline, cName, cProp, cValue)
}

func (p *Pipeline) setPropFloat(name string, prop string, value float32) {
	// fx prefix needed (added during pipeline initialization)
	cName := C.CString(name)
	cProp := C.CString(prop)
	cValue := C.float(value)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	C.gstSetPropFloat(p.cPipeline, cName, cProp, cValue)
}

func (p *Pipeline) SetEncodingRate(kind string, value64 uint64) {
	// see https://gstreamer.freedesktop.org/documentation/x264/index.html?gi-language=c#x264enc:bitrate
	// see https://gstreamer.freedesktop.org/documentation/nvcodec/GstNvBaseEnc.html?gi-language=c#GstNvBaseEnc:bitrate
	// see https://gstreamer.freedesktop.org/documentation/opus/opusenc.html?gi-language=c#opusenc:bitrate
	value := int(value64)
	prop := "bitrate"
	if kind == "audio" {
		p.setPropInt("audio_encoder_wet", prop, value)
	} else {
		names := []string{"video_encoder_dry", "video_encoder_wet"}
		if p.join.VideoFormat == "VP8" {
			// see https://gstreamer.freedesktop.org/documentation/vpx/GstVPXEnc.html?gi-language=c#GstVPXEnc:target-bitrate
			prop = "target-bitrate"
		} else if p.join.VideoFormat == "H264" {
			// in kbit/s for x264enc and nvh264enc
			value = value / 1000
		}
		for _, n := range names {
			p.setPropInt(n, prop, value)
		}
	}
}

func (p *Pipeline) SetFxProp(name string, prop string, value float32) {
	// fx prefix needed (added during pipeline initialization)
	p.setPropFloat("client_"+name, prop, value)
}

func (p *Pipeline) GetFxProp(name string, prop string) float32 {
	// fx prefix needed (added during pipeline initialization)
	cName := C.CString("client_" + name)
	cProp := C.CString(prop)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	return float32(C.gstGetPropFloat(p.cPipeline, cName, cProp))
}

func (p *Pipeline) SetFxPolyProp(name string, prop string, kind string, value string) {
	cName := C.CString("client_" + name)
	cProp := C.CString(prop)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	switch kind {
	case "float":
		if v, err := strconv.ParseFloat(value, 32); err == nil {
			cValue := C.float(float32(v))
			C.gstSetPropFloat(p.cPipeline, cName, cProp, cValue)
		}
	case "double":
		if v, err := strconv.ParseFloat(value, 64); err == nil {
			cValue := C.double(v)
			C.gstSetPropDouble(p.cPipeline, cName, cProp, cValue)
		}
	case "int":
		if v, err := strconv.ParseInt(value, 10, 32); err == nil {
			cValue := C.int(int32(v))
			C.gstSetPropInt(p.cPipeline, cName, cProp, cValue)
		}
	case "uint64":
		if v, err := strconv.ParseInt(value, 10, 64); err == nil {
			cValue := C.ulong(v)
			C.gstSetPropUint64(p.cPipeline, cName, cProp, cValue)
		}
	}
}
