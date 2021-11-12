package main

import (
	"os"

	"github.com/creamlab/ducksoup/front"
	"github.com/creamlab/ducksoup/gst"
	"github.com/creamlab/ducksoup/helpers"
	"github.com/creamlab/ducksoup/server"
	"github.com/rs/zerolog"
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

	// init logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "20060102-150405.000"})
	log.Logger = log.With().Caller().Logger()
}

func main() {
	// always build front (in watch mode or not, depending on DS_ENV value, see front/build.go)
	front.Build()

	// run ducksoup only if not in BUILD_FRONT DS_ENV
	if !cmdBuildMode {
		defer func() {
			log.Info().Msg("[main] stopped")
			if r := recover(); r != nil {
				log.Info().Msgf("[recov] main has recovered: %v", r)
			}
		}()

		// launch http (with websockets) server
		go server.ListenAndServe()

		// start Glib main loop for GStreamer
		gst.StartMainLoop()
	}
}
