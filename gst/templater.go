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
	"text/template"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/types"
)

func newPipelineDef(join types.JoinPayload, filePrefix string, videoOptions, audioOptions mediaOptions) string {

	// shape template data
	data := struct {
		// fields available for interpolation in template file
		Queue      queueConfig
		Video      mediaOptions
		Audio      mediaOptions
		Folder     string
		FilePrefix string
		Width      int
		Height     int
		Framerate  int
	}{
		gstConfig.Shared.Queue,
		videoOptions,
		audioOptions,
		join.DataFolder(),
		filePrefix,
		join.Width,
		join.Height,
		join.Framerate,
	}

	// render pipeline from template
	var buf bytes.Buffer
	var templater *template.Template
	if join.AudioOnly {
		if env.NoRecording {
			templater = audioOnlyNoRecordingTemplater
		} else if join.RecordingMode == "passthrough" {
			templater = audioOnlyPassthroughTemplater
		} else {
			// audio only default
			templater = audioOnlyTemplater
		}
	} else {
		if env.NoRecording {
			templater = noRecordingTemplater
		} else if join.RecordingMode == "split" {
			templater = splitTemplater
		} else if join.RecordingMode == "passthrough" {
			templater = passthroughTemplater
		} else if join.RecordingMode == "none" {
			templater = noRecordingTemplater
		} else if join.RecordingMode == "reenc" {
			templater = muxedReencTemplater
		} else {
			// audio+video default, ideally would be muxedTemplater
			templater = muxedRtpBinTemplater
			if join.VideoFormat == "VP8" { // if we switch default to muxedTemplater, keep reenc for VP8
				templater = muxedReencTemplater
			}
		}
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
