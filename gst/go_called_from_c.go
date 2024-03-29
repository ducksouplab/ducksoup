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

	"github.com/ducksouplab/ducksoup/env"
	extLogger "github.com/ducksouplab/ducksoup/logger"
	"github.com/ducksouplab/ducksoup/types"
	"github.com/rs/zerolog/log"
)

const GstLevelError = 1
const GstLevelWarning = 2

var (
	idRegexp, idsRegexp, frameRegexp, trackingRegexp *regexp.Regexp
)

func init() {
	idRegexp = regexp.MustCompile(`user-id: (.*?),`)
	idsRegexp = regexp.MustCompile(`r-(.*?)-u-(.*?)$`)
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
		p.plir.PLIRequest("gstreamer_request_key_frame")
	}
}

//export goBusLog
func goBusLog(cId, cMsg, cEl *C.char) {
	id := C.GoString(cId)
	msg := C.GoString(cMsg)
	el := C.GoString(cEl)
	p, found := pipelineStoreSingleton.find(id)

	if found {
		p.logger.Error().Err(errors.New(msg)).Str("element", el).Msg("gstreamer_error")
	} else {
		log.Error().Str("context", "gst").Err(errors.New(msg)).Str("element", el).Str("id", id).Msg("gstreamer_error_detached")
	}
}

//export goDebugLog
func goDebugLog(cLevel C.int, cFile, cFunction *C.char, line C.int, cMsg *C.char) {
	level := int(cLevel)
	from := "GStreamer:" + C.GoString(cFile) + ":" + C.GoString(cFunction) + ":" + strconv.Itoa(int(line))
	msg := C.GoString(cMsg)

	if env.GSTTracking && level == GstLevelWarning {
		match := idRegexp.FindStringSubmatch(msg)
		if len(match) > 0 {
			idsMatch := idsRegexp.FindStringSubmatch(match[1])
			if len(idsMatch) > 2 {
				if logger, ok := extLogger.GetLogger(idsMatch[1]); ok {
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
					logger.Warn().
						Str("context", "gstreamer").
						Int("GST_LEVEL", level).
						Str("user", idsMatch[2]).
						Str("frame", frame).
						Bool("value", tracking).
						Msg("video_tracking")
				}
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
