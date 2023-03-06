package main

import (
	"fmt"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/front"
	"github.com/ducksouplab/ducksoup/gst"
	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/ducksouplab/ducksoup/server"
	"github.com/rs/zerolog/log"
)

var (
	cmdBuildMode bool = false
)

func init() {
	if env.Mode == "FRONT_BUILD" {
		cmdBuildMode = true
	}

	helpers.EnsureDir("./data")

}

func main() {
	// always build front (in watch mode or not, depending on env.Mode value, see front/build.go)
	front.Build()

	// run ducksoup only if not in FRONT_BUILD env.Mode
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
