package main

import (
	"log"

	"github.com/creamlab/ducksoup/front"
	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/helpers"
	"github.com/creamlab/ducksoup/server"
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

	defer func() {
		if r := recover(); r != nil {
			log.Println(">>>> Recovered in main", r)
		}
	}()
}
