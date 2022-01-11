package main

import (
	"fmt"
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
				log.Error().Str("context", "app").Err(fmt.Errorf("%v", r)).Msg("app_panicked")
			}
			log.Info().Str("context", "app").Msg("app_ended")
		}()

		// launch http (with websockets) server
		go server.ListenAndServe()
		log.Info().Str("context", "app").Msg("app_started")

		// start Glib main loop for GStreamer
		gst.StartMainLoop()
	}
}
