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

	"github.com/ducksouplab/ducksoup/types"
)

func newPipelineDef(join types.JoinPayload, filePrefix string, videoOptions, audioOptions mediaOptions) string {

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
