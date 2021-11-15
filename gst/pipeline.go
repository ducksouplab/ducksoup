// Package gst provides an easy API to create a GStreamer pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"os"
	"strings"
	"unsafe"

	_ "github.com/creamlab/ducksoup/helpers" // rely on helpers logger init side-effect
	"github.com/creamlab/ducksoup/types"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// global state
var (
	nvidia bool
)

func init() {
	nvidia = strings.ToLower(os.Getenv("DS_NVIDIA")) == "true"
}

// Pipeline is a wrapper for a GStreamer pipeline and output track
type Pipeline struct {
	id          string // same as local/output track id
	join        types.JoinPayload
	cPipeline   *C.GstElement
	audioOutput types.TrackWriter
	videoOutput types.TrackWriter
	filePrefix  string
	pliCallback func()
	// log
	logger zerolog.Logger
}

func fileName(namespace string, prefix string, suffix string) string {
	return namespace + "/" + prefix + "-" + suffix + ".mkv"
}

//export goStopCallback
func goStopCallback(cId *C.char) {
	id := C.GoString(cId)
	pipelines.delete(id)
}

func writeNewSample(kind string, cId *C.char, buffer unsafe.Pointer, bufferLen C.int) {
	id := C.GoString(cId)
	p, ok := pipelines.find(id)

	if ok {
		var output types.TrackWriter
		if kind == "audio" {
			output = p.audioOutput
		} else {
			output = p.videoOutput
		}

		buf := C.GoBytes(buffer, bufferLen)
		if err := output.Write(buf); err != nil {
			// TODO err contains the ID of the failing PeerConnections
			// we may store a callback on the Pipeline struct (the callback would remove those peers and update signaling)
			p.logger.Error().Err(err).Msg("can't write to track")
		}
	} else {
		// TODO return error to gst.c and stop processing?
		p.logger.Error().Msg("pipeline not found, discarding buffer")
	}
	C.free(buffer)
}

//export goAudioCallback
func goAudioCallback(cId *C.char, buffer unsafe.Pointer, bufferLen C.int, pts C.int) {
	writeNewSample("audio", cId, buffer, bufferLen)
}

//export goVideoCallback
func goVideoCallback(cId *C.char, buffer unsafe.Pointer, bufferLen C.int, pts C.int) {
	writeNewSample("video", cId, buffer, bufferLen)
}

//export goPLICallback
func goPLICallback(cId *C.char) {
	id := C.GoString(cId)
	p, ok := pipelines.find(id)

	if ok {
		p.logger.Info().Msg("PLI requested from GStreamer")
		p.pliCallback()
	}
}

// API

func StartMainLoop() {
	C.gstreamer_start_mainloop()
}

// create a GStreamer pipeline
func CreatePipeline(join types.JoinPayload, filePrefix string) *Pipeline {

	pipelineStr := newPipelineDef(join, filePrefix)
	id := uuid.New().String()

	cPipelineStr := C.CString(pipelineStr)
	cId := C.CString(id)
	defer C.free(unsafe.Pointer(cPipelineStr))
	defer C.free(unsafe.Pointer(cId))

	logger := log.With().
		Str("room", join.RoomId).
		Str("user", join.UserId).
		Str("pipeline", id).
		Logger()

	p := &Pipeline{
		id:         id,
		join:       join,
		cPipeline:  C.gstreamer_parse_pipeline(cPipelineStr, cId),
		filePrefix: filePrefix,
		logger:     logger,
	}

	p.logger.Info().Str("pipeline", pipelineStr).Msg("pipeline initialized")

	pipelines.add(p)
	return p
}

func (p *Pipeline) outputFiles() []string {
	if p.join.AudioFx == "passthrough" && p.join.VideoFx == "passthrough" {
		return nil
	}
	namespace := p.join.Namespace
	hasFx := (len(p.join.AudioFx) > 0 && p.join.AudioFx != "passthrough") || (len(p.join.VideoFx) > 0 && p.join.VideoFx != "passthrough")
	if hasFx {
		return []string{fileName(namespace, p.filePrefix, "raw"), fileName(namespace, p.filePrefix, "fx")}
	} else {
		return []string{fileName(namespace, p.filePrefix, "raw")}
	}
}

func (p *Pipeline) pushSample(src string, buffer []byte) {
	s := C.CString(src)
	defer C.free(unsafe.Pointer(s))

	b := C.CBytes(buffer)
	defer C.free(b)
	C.gstreamer_push_buffer(s, p.cPipeline, b, C.int(len(buffer)))
}

func (p *Pipeline) BindTrack(kind string, t types.TrackWriter) (f types.PushFunc, files []string) {
	if kind == "audio" {
		p.audioOutput = t
	} else {
		p.videoOutput = t
	}
	if p.audioOutput != nil && p.videoOutput != nil {
		p.start()
		files = p.outputFiles()
	}
	f = func(b []byte) {
		p.pushSample(kind+"_src", b)
	}
	return
}

func (p *Pipeline) BindPLICallback(c func()) {
	p.pliCallback = c
}

// start the GStreamer pipeline
func (p *Pipeline) start() {
	C.gstreamer_start_pipeline(p.cPipeline)
	p.logger.Info().Msgf("pipeline started with recording prefix: %s/%s", p.join.Namespace, p.filePrefix)
}

// stop the GStreamer pipeline
func (p *Pipeline) Stop() {
	C.gstreamer_stop_pipeline(p.cPipeline)
	p.logger.Info().Msg("pipeline stop requested")
}

func (p *Pipeline) getPropertyInt(name string, prop string) int {
	cName := C.CString(name)
	cProp := C.CString(prop)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	return int(C.gstreamer_get_property_int(p.cPipeline, cName, cProp))
}

func (p *Pipeline) setPropertyInt(name string, prop string, value int) {
	// fx prefix needed (added during pipeline initialization)
	cName := C.CString(name)
	cProp := C.CString(prop)
	cValue := C.int(value)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	C.gstreamer_set_property_int(p.cPipeline, cName, cProp, cValue)
}

func (p *Pipeline) setPropertyFloat(name string, prop string, value float32) {
	// fx prefix needed (added during pipeline initialization)
	cName := C.CString(name)
	cProp := C.CString(prop)
	cValue := C.float(value)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	C.gstreamer_set_property_float(p.cPipeline, cName, cProp, cValue)
}

func (p *Pipeline) SetEncodingRate(kind string, value64 uint64) {
	// see https://gstreamer.freedesktop.org/documentation/x264/index.html?gi-language=c#x264enc:bitrate
	// see https://gstreamer.freedesktop.org/documentation/nvcodec/GstNvBaseEnc.html?gi-language=c#GstNvBaseEnc:bitrate
	// see https://gstreamer.freedesktop.org/documentation/opus/opusenc.html?gi-language=c#opusenc:bitrate
	value := int(value64)
	prop := "bitrate"
	if kind == "audio" {
		p.setPropertyInt("audio_encoder_fx", prop, value)
	} else {
		names := []string{"video_encoder_raw", "video_encoder_fx"}
		if p.join.VideoFormat == "VP8" {
			// see https://gstreamer.freedesktop.org/documentation/vpx/GstVPXEnc.html?gi-language=c#GstVPXEnc:target-bitrate
			prop = "target-bitrate"
		} else if p.join.VideoFormat == "H264" {
			// in kbit/s for x264enc and nvh264enc
			value = value / 1000
			if p.join.GPU {
				// acts both on bitrate and max-bitrate for nvh264enc
				for _, n := range names {
					p.setPropertyInt(n, "max-bitrate", value*320/256)
				}
			}
		}
		for _, n := range names {
			p.setPropertyInt(n, prop, value)
		}
	}
}

func (p *Pipeline) SetFxProperty(kind string, name string, prop string, value float32) {
	// fx prefix needed (added during pipeline initialization)
	p.setPropertyFloat(kind+"_fx_"+name, prop, value)
}

func (p *Pipeline) GetFxProperty(kind string, name string, prop string) float32 {
	// fx prefix needed (added during pipeline initialization)
	cName := C.CString(kind + "_fx_" + name)
	cProp := C.CString(prop)

	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cProp))

	return float32(C.gstreamer_get_property_float(p.cPipeline, cName, cProp))
}
