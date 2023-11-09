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
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/ducksouplab/ducksoup/config"
	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/types"
)

func newPipelineDef(jp types.JoinPayload, dataFolder, filePrefix string, videoOptions, audioOptions mediaOptions) string {

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
		RTPBin     string
		FinalQueue string
	}{
		gstConfig.Shared.Queue,
		videoOptions,
		audioOptions,
		dataFolder,
		filePrefix,
		jp.Width,
		jp.Height,
		jp.Framerate,
		"rtpbin name=rtpbin latency=" + strconv.Itoa(env.JitterBuffer),
		"queue max-size-buffers=0 max-size-bytes=0 max-size-time=" + strconv.Itoa(env.JitterBuffer) + "000000",
	}

	// render pipeline from template
	var buf bytes.Buffer
	var templater *template.Template
	if jp.AudioOnly {
		if env.NoRecording {
			templater = audioOnlyNoRecordingTemplater
		} else if jp.RecordingMode == "passthrough" {
			templater = audioOnlyPassthroughTemplater
		} else {
			// audio only default
			templater = audioOnlyTemplater
		}
	} else {
		if env.NoRecording {
			templater = noRecordingTemplater
		} else if jp.RecordingMode == "split" {
			templater = splitTemplater
		} else if jp.RecordingMode == "passthrough" {
			templater = passthroughTemplater
		} else if jp.RecordingMode == "none" {
			templater = noRecordingTemplater
		} else if jp.RecordingMode == "reenc" {
			templater = muxedReencDryTemplater
		} else if jp.RecordingMode == "free" {
			templater = muxedFreeFramerateTemplater
		} else { // default
			// audio+video default, ideally would be muxedTemplater
			templater = muxedForcedFramerateTemplater
			if jp.VideoFormat == "VP8" { // if we switch default to muxedTemplater, keep reenc for VP8
				templater = muxedReencDryTemplater
			}
		}
	}
	if err := templater.Execute(&buf, data); err != nil {
		panic(err)
	}

	// log pipeline
	contents := []byte("DuckSoup: " + config.BackendVersion + "\n\n")
	contents = append(contents, buf.Bytes()...)
	os.WriteFile(dataFolder+"/pipeline-"+jp.UserId+".txt", contents, 0666)

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
