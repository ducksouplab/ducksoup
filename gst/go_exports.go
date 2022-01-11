package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"strconv"
	"unsafe"

	"github.com/creamlab/ducksoup/types"
	"github.com/rs/zerolog/log"
)

const GstLevelError = 1
const GstLevelWarning = 2

// C exports

//export goDeletePipeline
func goDeletePipeline(cId *C.char) {
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

//export goWriteAudio
func goWriteAudio(cId *C.char, buffer unsafe.Pointer, bufferLen C.int, pts C.int) {
	writeNewSample("audio", cId, buffer, bufferLen)
}

//export goWriteVideo
func goWriteVideo(cId *C.char, buffer unsafe.Pointer, bufferLen C.int, pts C.int) {
	writeNewSample("video", cId, buffer, bufferLen)
}

//export goPLIRequest
func goPLIRequest(cId *C.char) {
	id := C.GoString(cId)
	p, ok := pipelines.find(id)

	if ok {
		p.logger.Info().Msg("PLI requested from GStreamer")
		p.pliCallback()
	}
}

//export goPipelineLog
func goPipelineLog(cId *C.char, msg *C.char, isError C.int) {
	id := C.GoString(cId)
	m := C.GoString(msg)
	p, ok := pipelines.find(id)

	if ok {
		if isError == 0 {
			p.logger.Error().Msg(m)
		} else {
			p.logger.Error().Msg(m)
		}
	}
}

//export goDebugLog
func goDebugLog(cLevel C.int, cFile, cFunction *C.char, line C.int, cMsg *C.char) {
	level := int(cLevel)
	from := "GStreamer:" + C.GoString(cFile) + ":" + C.GoString(cFunction) + ":" + strconv.Itoa(int(line))
	msg := C.GoString(cMsg)

	if level == GstLevelError {
		log.Error().Str("context", "gstreamer").Str("from", from).Int("GST_LEVEL", level).Msg(msg)
	} else if level == GstLevelWarning {
		log.Warn().Str("context", "gstreamer").Str("from", from).Int("GST_LEVEL", level).Msg(msg)
	} else {
		log.Info().Str("context", "gstreamer").Str("from", from).Int("GST_LEVEL", level).Msg(msg)
	}
}
