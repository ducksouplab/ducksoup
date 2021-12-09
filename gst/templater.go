// Package gst provides an easy API to create a GStreamer pipeline
package gst

/*
#cgo pkg-config: gstreamer-1.0 gstreamer-app-1.0
#include "gst.h"
*/
import "C"
import (
	"bufio"
	"bytes"
	"strings"

	"github.com/creamlab/ducksoup/types"
)

func newPipelineDef(join types.JoinPayload, filePrefix string) string {
	audioCodec := config.Opus
	// rely on the fact that assigning to a struct with only primitive values (string), is copying by value
	// caution: don't extend codec type with non primitive values
	if &audioCodec == &config.Opus {
		panic("Unhandled audioCodec assign")
	}
	// choose videoCodec
	var videoCodec codec
	switch join.VideoFormat {
	case "VP8":
		videoCodec = config.VP8
	case "H264":
		if nvidiaEnabled && join.GPU {
			videoCodec = config.NV264
		} else {
			videoCodec = config.X264
		}
	default:
		panic("Unhandled format " + join.VideoFormat)
	}
	// complete with Fx
	audioCodec.Fx = strings.Replace(join.AudioFx, "name=", "name=video_fx_", 1)
	videoCodec.Fx = strings.Replace(join.VideoFx, "name=", "name=audio_fx_", 1)

	// shape template data
	data := struct {
		Video      codec
		Audio      codec
		Namespace  string
		FilePrefix string
		Width      int
		Height     int
		FrameRate  int
	}{
		videoCodec,
		audioCodec,
		join.Namespace,
		filePrefix,
		join.Width,
		join.Height,
		join.FrameRate,
	}

	// render pipeline from template
	var buf bytes.Buffer
	templater := muxedRecordingTemplater
	if join.RecordingMode == "split" {
		templater = splitRecordingTemplater
	} else if join.RecordingMode == "passthrough" {
		templater = passthroughTemplater
	} else if join.RecordingMode == "none" {
		templater = noRecordingTemplater
	}
	if err := templater.Execute(&buf, data); err != nil {
		panic(err)
	}

	// process lines (trim and remove blank lines)
	var formattedBuf bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader(buf.String()))
	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if len(trimmed) > 0 {
			formattedBuf.WriteString(trimmed + "\n")
		}
	}

	return formattedBuf.String()
}
