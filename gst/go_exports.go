package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"errors"
	"regexp"
	"strconv"
	"unsafe"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/creamlab/ducksoup/types"
	"github.com/rs/zerolog/log"
)

const GstLevelError = 1
const GstLevelWarning = 2

var (
	enableLogTracking                                bool
	idRegexp, idsRegexp, frameRegexp, trackingRegexp *regexp.Regexp
)

func init() {
	enableLogTracking = false
	if helpers.Getenv("DS_GST_ENABLE_TRACKING") == "true" {
		enableLogTracking = true
	}
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

func writeNewSample(kind string, cId *C.char, buffer unsafe.Pointer, bufferLen C.int) {
	id := C.GoString(cId)
	p, ok := pipelineStoreSingleton.find(id)

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
func goWriteAudio(cId *C.char, buffer unsafe.Pointer, bufferLen C.int, pts C.int) {
	writeNewSample("audio", cId, buffer, bufferLen)
}

//export goWriteVideo
func goWriteVideo(cId *C.char, buffer unsafe.Pointer, bufferLen C.int, pts C.int) {
	writeNewSample("video", cId, buffer, bufferLen)
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
			p.logger.Log().Err(errors.New(m)).Msg("gstreamer_pipeline_log")
		}
	}
}

//export goDebugLog
func goDebugLog(cLevel C.int, cFile, cFunction *C.char, line C.int, cMsg *C.char) {
	level := int(cLevel)
	from := "GStreamer:" + C.GoString(cFile) + ":" + C.GoString(cFunction) + ":" + strconv.Itoa(int(line))
	msg := C.GoString(cMsg)

	if enableLogTracking && level == GstLevelWarning {
		match := idRegexp.FindStringSubmatch(msg)
		if len(match) > 0 {
			idsMatch := idsRegexp.FindStringSubmatch(match[1])
			if len(idsMatch) > 3 {
				frameMatch := frameRegexp.FindStringSubmatch(msg)
				frame := ""
				if len(frameMatch) > 0 {
					frame = frameMatch[1]
				}
				trackingMatch := trackingRegexp.FindStringSubmatch(msg)
				tracking := false
				if len(trackingMatch) > 0 {
					tracking = true
				}
				log.Warn().
					Str("context", "gstreamer").
					Int("GST_LEVEL", level).
					Str("namespace", idsMatch[1]).
					Str("room", idsMatch[2]).
					Str("user", idsMatch[3]).
					Str("frame", frame).
					Bool("value", tracking).
					Msg("video_tracking")
			}
		} else {
			log.Warn().Str("context", "gstreamer").Str("from", from).Int("GST_LEVEL", level).Msg(msg)
		}
	} else if level == GstLevelError {
		log.Error().Str("context", "gstreamer").Str("from", from).Int("GST_LEVEL", level).Msg(msg)
	} else if level == GstLevelWarning {
		log.Warn().Str("context", "gstreamer").Str("from", from).Int("GST_LEVEL", level).Msg(msg)
	} else {
		log.Info().Str("context", "gstreamer").Str("from", from).Int("GST_LEVEL", level).Msg(msg)
	}
}
