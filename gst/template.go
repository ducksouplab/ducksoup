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
	"time"

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
		"rtpbin name=rtpbin latency=200", // + strconv.Itoa(env.JitterBuffer),
		// important: max-size-time greater than the jitter buffer latency to prevent audio glitches
		"queue max-size-buffers=0 max-size-bytes=0 max-size-time=" + strconv.Itoa(env.JitterBuffer+100) + "000000",
	}

	// render pipeline from template
	var buf bytes.Buffer
	var templateName string
	if jp.AudioOnly {
		if env.NoRecording {
			templateName = "audio_only_no_recording"
		} else {
			// audio only default
			templateName = "audio_only"
		}
	} else {
		if env.NoRecording {
			templateName = "no_recording"
		} else if jp.RecordingMode == "split" {
			templateName = "split"
		} else if jp.RecordingMode == "rtpbin_only" {
			templateName = "rtpbin_only"
		} else if jp.RecordingMode == "none" {
			templateName = "no_recording"
		} else if jp.RecordingMode == "reenc" {
			templateName = "muxed_reenc_dry"
		} else if jp.RecordingMode == "free" {
			templateName = "muxed_free_framerate"
		} else { // default
			// audio+video default, ideally would be muxedTemplater
			templateName = "muxed_forced_framerate"
			if jp.VideoFormat == "VP8" { // if we switch default to muxedTemplater, keep reenc for VP8
				templateName = "muxed_reenc_dry"
			}
		}
	}
	template := templateIndex[templateName]
	if err := template.Execute(&buf, data); err != nil {
		panic(err)
	}

	// log pipeline
	if jp.RecordingMode != "bypass" {
		contents := []byte("// DuckSoup#" + config.BackendVersion + " Pipeline#" + templateName + "\n\n")
		contents = append(contents, buf.Bytes()...)
		os.WriteFile(dataFolder+"/pipeline-u-"+jp.UserId+"-"+time.Now().Format("20060102-150405.000")+".txt", contents, 0666)
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
