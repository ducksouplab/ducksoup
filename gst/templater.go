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
	"strconv"
	"strings"

	"github.com/creamlab/ducksoup/types"
)

func newPipelineDef(join types.JoinPayload, filePrefix string) string {
	// choose videoCodec
	var videoCodec codec
	switch join.VideoFormat {
	case "VP8":
		videoCodec = config.VP8
	case "H264":
		if nvidia && join.GPU {
			videoCodec = config.NV264
		} else {
			videoCodec = config.X264
		}
	default:
		panic("Unhandled format " + join.VideoFormat)
	}
	// ugly but needs to be replaced here, since videoCodec is used as a raw value in the templater
	// (and not something to be interpolated)
	videoCodec.Encode.Raw = strings.Replace(videoCodec.Encode.Raw, "{{.Namespace}}", join.Namespace, -1)
	videoCodec.Encode.Raw = strings.Replace(videoCodec.Encode.Raw, "{{.FilePrefix}}", filePrefix, -1)
	videoCodec.Encode.Fx = strings.Replace(videoCodec.Encode.Fx, "{{.Namespace}}", join.Namespace, -1)
	videoCodec.Encode.Fx = strings.Replace(videoCodec.Encode.Fx, "{{.FilePrefix}}", filePrefix, -1)
	// prepare width, height, framerate
	width := ", width=" + strconv.Itoa(join.Width)
	height := ", height=" + strconv.Itoa(join.Height)
	frameRate := ", framerate=" + strconv.Itoa(join.FrameRate) + "/1"

	// shape template data
	data := struct {
		Video           codec
		Audio           codec
		Namespace       string
		FilePrefix      string
		RTPJitterBuffer rtpJitterBuffer
		AudioFx         string
		VideoFx         string
		Width           string
		Height          string
		FrameRate       string
	}{
		videoCodec,
		config.Opus,
		join.Namespace,
		filePrefix,
		config.RTPJitterBuffer,
		strings.Replace(join.AudioFx, "name=", "name=audio_fx_", 1), // add prefix to avoid clash with other named elements
		strings.Replace(join.VideoFx, "name=", "name=video_fx_", 1),
		width,
		height,
		frameRate,
	}

	// render pipeline from template
	var buf bytes.Buffer
	if err := pipelineTemplater.Execute(&buf, data); err != nil {
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
