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

	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/types"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

// Pipeline is a wrapper for a GStreamer pipeline and output track
type Pipeline struct {
	mu          sync.Mutex
	id          string // same as local/output track id
	join        types.JoinPayload
	cPipeline   *C.GstElement
	audioOutput types.TrackWriter
	videoOutput types.TrackWriter
	filePrefix  string
	// options
	videoOptions mediaOptions
	audioOptions mediaOptions
	// stoppedCount=2 if audio and video have been stopped
	stoppedCount int
	// log
	logger zerolog.Logger
	// API
	ReadyCh chan struct{}
}

func fileName(namespace string, prefix string, suffix string) string {
	return namespace + "/" + prefix + "-" + suffix + ".mkv"
}

func getOptions(join types.JoinPayload) (videoOptions, audioOptions mediaOptions) {
	audioOptions = gstConfig.Opus
	// rely on the fact that assigning to a struct with only primitive values (string), is copying by value
	// caution: don't extend codec type with non primitive values
	if &audioOptions == &gstConfig.Opus {
		panic("Unhandled audioCodec assign")
	}
	// choose videoCodec
	nvCodec := env.NVCodec && join.GPU
	nvCuda := env.NVCuda && join.GPU
	switch join.VideoFormat {
	case "VP8":
		videoOptions = gstConfig.VP8
		videoOptions.SkipFixedCaps = true
	case "H264":
		if nvCodec {
			videoOptions = gstConfig.NV264
		} else {
			videoOptions = gstConfig.X264
		}
	default:
		panic("Unhandled format " + join.VideoFormat)
	}
	// set env and join dependent options
	videoOptions.nvCodec = nvCodec
	videoOptions.nvCuda = nvCuda
	videoOptions.Overlay = join.Overlay || env.ForceOverlay
	// complete with Fx
	audioOptions.Fx = strings.Replace(join.AudioFx, "name=", "name=client_", -1)
	videoOptions.Fx = strings.Replace(join.VideoFx, "name=", "name=client_", -1)

	return
}

// API

func StartMainLoop() {
	C.gstStartMainLoop()
}

// create a GStreamer pipeline
func NewPipeline(join types.JoinPayload, filePrefix string, logger zerolog.Logger) *Pipeline {
	id := uuid.New().String()
	logger = logger.With().
		Str("context", "pipeline").
		Str("user", join.UserId).
		Str("pipeline", id).
		Logger()

	videoOptions, audioOptions := getOptions(join)
	logger.Info().Str("audioOptions", fmt.Sprintf("%+v", audioOptions)).Msg("template_data")
	logger.Info().Str("videoOptions", fmt.Sprintf("%+v", videoOptions)).Msg("template_data")

	pipelineStr := newPipelineDef(join, filePrefix, videoOptions, audioOptions)

	cPipelineStr := C.CString(pipelineStr)
	cId := C.CString(id)
	defer C.free(unsafe.Pointer(cPipelineStr))
	defer C.free(unsafe.Pointer(cId))

	p := &Pipeline{
		mu:           sync.Mutex{},
		id:           id,
		join:         join,
		cPipeline:    C.gstParsePipeline(cPipelineStr, cId),
		filePrefix:   filePrefix,
		videoOptions: videoOptions,
		audioOptions: audioOptions,
		stoppedCount: 0,
		ReadyCh:      make(chan struct{}),
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

func (p *Pipeline) srcPush(src string, buffer []byte) {
	s := C.CString(src)
	defer C.free(unsafe.Pointer(s))

	b := C.CBytes(buffer)
	defer C.free(b)
	C.gstSrcPush(s, p.cPipeline, b, C.int(len(buffer)))
}

func (p *Pipeline) PushRTP(kind string, buffer []byte) {
	p.srcPush(kind+"_src", buffer)
}

func (p *Pipeline) PushRTCP(kind string, buffer []byte) {
	p.srcPush(kind+"_rtcp_src", buffer)
}

func (p *Pipeline) BindTrackAutoStart(kind string, t types.TrackWriter) {
	if kind == "audio" {
		p.audioOutput = t
	} else {
		p.videoOutput = t
	}
	p.updateReady()
}

func (p *Pipeline) updateReady() {
	if p.audioOutput != nil && p.videoOutput != nil {
		close(p.ReadyCh)
		p.Start()
	}
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

func (p *Pipeline) setPropString(name, prop, value string) {
	cName := C.CString(name)
	cProp := C.CString(prop)
	cValue := C.CString(value)
	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))
	defer C.free(unsafe.Pointer(cValue))

	C.gstSetPropString(p.cPipeline, cName, cProp, cValue)
}

func (p *Pipeline) SetEncodingBitrate(kind string, value int) {
	// see https://gstreamer.freedesktop.org/documentation/x264/index.html?gi-language=c#x264enc:bitrate
	// see https://gstreamer.freedesktop.org/documentation/nvcodec/GstNvBaseEnc.html?gi-language=c#GstNvBaseEnc:bitrate
	// see https://gstreamer.freedesktop.org/documentation/opus/opusenc.html?gi-language=c#opusenc:bitrate
	if kind == "audio" {
		p.setPropInt("audio_encoder_wet", "bitrate", value)
	} else {
		if p.join.VideoFormat == "VP8" {
			// see https://gstreamer.freedesktop.org/documentation/vpx/GstVPXEnc.html?gi-language=c#GstVPXEnc:target-bitrate
			p.setPropInt("video_encoder_dry", "target-bitrate", value)
			p.setPropInt("video_encoder_wet", "target-bitrate", value)
		} else if p.join.VideoFormat == "H264" {
			// in kbit/s for x264enc and nvh264enc
			value = value / 1000
			p.setPropInt("video_encoder_dry", "bitrate", value)
			p.setPropInt("video_encoder_wet", "bitrate", value)
			if p.videoOptions.nvCodec {
				// https://gstreamer.freedesktop.org/documentation/nvcodec/GstNvBaseEnc.html?gi-language=c#GstNvBaseEnc:max-bitrate
				p.setPropInt("video_encoder_dry", "max-bitrate", value*280/256)
				p.setPropInt("video_encoder_wet", "max-bitrate", value*280/256)
			}
		}
	}
}

func (p *Pipeline) SetFxPropFloat(name string, prop string, value float32) {
	// fx prefix needed (added during pipeline initialization)
	p.setPropFloat("client_"+name, prop, value)
}

func (p *Pipeline) GetFxPropFloat(name string, prop string) float32 {
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
	case "string":
		cValue := C.CString(value)
		defer C.free(unsafe.Pointer(cValue))

		C.gstSetPropString(p.cPipeline, cName, cProp, cValue)
	}
}
