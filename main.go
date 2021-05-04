package main

import (
	"github.com/creamlab/webrtc-transform/front"
	"github.com/creamlab/webrtc-transform/gst"
	"github.com/creamlab/webrtc-transform/helpers"
	"github.com/creamlab/webrtc-transform/server"
)

func init() {
	helpers.EnsureDir("./logs")
}

func main() {
	// build front
	front.Build()

	// launch http (with websockets) server
	go server.ListenAndServe()

	// start Glib main loop for GStreamer
	gst.StartMainLoop()
}
