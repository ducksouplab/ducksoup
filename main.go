package main

import (
	"os"

	"github.com/creamlab/ducksoup/front"
	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/helpers"
	"github.com/creamlab/ducksoup/server"
	"github.com/rs/zerolog/log"
)

var (
	cmdBuildMode bool = false
)

func init() {

	if os.Getenv("DS_ENV") == "BUILD_FRONT" {
		cmdBuildMode = true
	}

	helpers.EnsureDir("./data")
}

func main() {
	// always build front (in watch mode or not, depending on DS_ENV value, see front/build.go)
	front.Build()

	// run ducksoup only if not in BUILD_FRONT DS_ENV
	if !cmdBuildMode {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Msgf("[main] app panic caught: %v", r)
			}
			log.Info().Msg("[main] app stopped")
		}()

		// launch http (with websockets) server
		go server.ListenAndServe()

		// start Glib main loop for GStreamer
		gst.StartMainLoop()
	}
}
