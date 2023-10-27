package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"errors"
	"regexp"
	"unsafe"

	"github.com/ducksouplab/ducksoup/types"
)

const GstLevelError = 1
const GstLevelWarning = 2

var (
	idRegexp, idsRegexp, frameRegexp, trackingRegexp *regexp.Regexp
)

func init() {
	idRegexp = regexp.MustCompile(`user-id: (.*?),`)
	idsRegexp = regexp.MustCompile(`n-(.*?)-r-(.*?)-u-(.*?)$`)
	frameRegexp = regexp.MustCompile(`frame: (.*?),`)
	trackingRegexp = regexp.MustCompile("TRACKER_OK")
}

// C exports

//export goDeletePipeline
func goDeletePipeline(cId *C.char) {
	id := C.GoString(cId)
	pipelineStoreSingleton.delete(id)
}

func writeTo(kind string, cId *C.char, buffer unsafe.Pointer, bufferLen C.int) {
	id := C.GoString(cId)
	p, ok := pipelineStoreSingleton.find(id)

	if ok {
		var output types.TrackWriter
		if kind == "audio" {
			output = p.audioOutput
		} else {
			output = p.videoOutput
			// p.logger.Debug().Int("value", int(bufferLen)).Msg("rtp_buffer_length")
		}

		buf := C.GoBytes(buffer, bufferLen)
		if err := output.Write(buf); err != nil {
			// TODO err contains the ID of the failing PeerConnections
			// we may store a callback on the Pipeline struct (the callback would remove those peers and update signaling)
			p.logger.Error().Err(err).Msg("track_write_failed")
		}
	} else {
		// discards buffer
		// TODO return error to gst.c and stop processing?
		p.logger.Error().Msg("pipeline_not_found")
	}
	C.free(buffer)
}

//export goWriteAudio
func goWriteAudio(cId *C.char, buffer unsafe.Pointer, bufferLen C.int) {
	writeTo("audio", cId, buffer, bufferLen)
}

//export goWriteVideo
func goWriteVideo(cId *C.char, buffer unsafe.Pointer, bufferLen C.int) {
	writeTo("video", cId, buffer, bufferLen)
}

//export goRequestKeyFrame
func goRequestKeyFrame(cId *C.char) {
	id := C.GoString(cId)
	p, ok := pipelineStoreSingleton.find(id)

	if ok {
		p.logger.Log().Msg("gstreamer_request_key_frame")
	}
}

//export goPipelineLog
func goPipelineLog(cId *C.char, msg *C.char, isError C.int) {
	id := C.GoString(cId)
	m := C.GoString(msg)
	p, ok := pipelineStoreSingleton.find(id)

	if ok {
		if isError == 0 {
			p.logger.Error().Err(errors.New(m)).Msg("gstreamer_pipeline_error")
		} else {
			// CAUTION: not documented
			p.logger.Log().Str("value", m).Msg("gstreamer_pipeline_log")
		}
	}
}
