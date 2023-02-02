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
	"fmt"
	"strings"

	"github.com/ducksouplab/ducksoup/types"
	"github.com/rs/zerolog/log"
)

func newPipelineDef(join types.JoinPayload, filePrefix string) string {
	audioOptions := config.Opus
	// rely on the fact that assigning to a struct with only primitive values (string), is copying by value
	// caution: don't extend codec type with non primitive values
	if &audioOptions == &config.Opus {
		panic("Unhandled audioCodec assign")
	}
	// choose videoCodec
	var videoOptions mediaOptions
	nvcodec := nvcodecEnv && join.GPU
	switch join.VideoFormat {
	case "VP8":
		videoOptions = config.VP8
		videoOptions.SkipFixedCaps = true
	case "H264":
		if nvcodec {
			videoOptions = config.NV264
		} else {
			videoOptions = config.X264
		}
	default:
		panic("Unhandled format " + join.VideoFormat)
	}
	// set env and join dependent options
	videoOptions.nvcodec = nvcodec
	videoOptions.Overlay = join.Overlay
	// complete with Fx
	audioOptions.Fx = strings.Replace(join.AudioFx, "name=", "name=client_", -1)
	videoOptions.Fx = strings.Replace(join.VideoFx, "name=", "name=client_", -1)

	log.Info().Str("context", "pipeline").Str("audioOptions", fmt.Sprintf("%+v", audioOptions)).Msg("template_data")
	log.Info().Str("context", "pipeline").Str("videoOptions", fmt.Sprintf("%+v", videoOptions)).Msg("template_data")

	// shape template data
	data := struct {
		Video      mediaOptions
		Audio      mediaOptions
		Namespace  string
		FilePrefix string
		Width      int
		Height     int
		FrameRate  int
	}{
		videoOptions,
		audioOptions,
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
