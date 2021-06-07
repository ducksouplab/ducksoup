package main

import (
	"log"
	"os"

	"github.com/creamlab/ducksoup/front"
	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/helpers"
	"github.com/creamlab/ducksoup/server"
)

var (
	cmdBuildMode bool = false
)

func init() {
	if os.Getenv("DS_ENV") == "BUILD_FRONT" {
		cmdBuildMode = true
	}
}

func init() {
	helpers.EnsureDir("./logs")
}

func main() {
	// always build front (in watch mode or not, depending on DS_ENV value, see front/build.go)
	front.Build()

	// run ducksoup only if not in BUILD_FRONT DS_ENV
	if !cmdBuildMode {
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
}
