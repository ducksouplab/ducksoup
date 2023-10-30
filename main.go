package main

import (
	"fmt"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/frontbuild"
	"github.com/ducksouplab/ducksoup/gst"
	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/ducksouplab/ducksoup/iceservers"
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

func logState() {
	log.Info().Str("context", "init").Msg("app_started")
	log.Info().Str("context", "init").Str("value", env.Mode).Msg("DUCKSOUP_MODE")
	log.Info().Str("context", "init").Str("value", env.Port).Msg("DUCKSOUP_PORT")
	log.Info().Str("context", "init").Str("value", env.WebPrefix).Msg("DUCKSOUP_WEB_PREFIX")
	log.Info().Str("context", "init").Str("value", env.PublicIP).Msg("DUCKSOUP_PUBLIC_IP")
	log.Info().Str("context", "init").Str("value", fmt.Sprintf("%v", env.AllowedWSOrigins)).Msg("DUCKSOUP_ALLOWED_WS_ORIGINS")
	log.Info().Str("context", "init").Bool("value", env.ExplicitHostCandidate).Msg("DUCKSOUP_EXPLICIT_HOST_CANDIDATE")
	log.Info().Str("context", "init").Bool("value", env.NVCodec).Msg("DUCKSOUP_NVCODEC")
	log.Info().Str("context", "init").Bool("value", env.NVCuda).Msg("DUCKSOUP_NVCUDA")
	log.Info().Str("context", "init").Int("value", env.JitterBuffer).Msg("DUCKSOUP_JITTER_BUFFER")
	log.Info().Str("context", "init").Bool("value", env.GeneratePlots).Msg("DUCKSOUP_GENERATE_PLOTS")
	log.Info().Str("context", "init").Bool("value", env.GenerateTWCC).Msg("DUCKSOUP_GENERATE_TWCC")
	log.Info().Str("context", "init").Bool("value", env.GCC).Msg("DUCKSOUP_GCC")
	log.Info().Str("context", "init").Bool("value", env.GSTTracking).Msg("DUCKSOUP_GST_TRACKING")
	log.Info().Str("context", "init").Int("value", env.LogLevel).Msg("DUCKSOUP_LOG_LEVEL")
	log.Info().Str("context", "init").Str("value", env.LogFile).Msg("DUCKSOUP_LOG_FILE")
	log.Info().Str("context", "init").Bool("value", env.ForceOverlay).Msg("DUCKSOUP_FORCE_OVERLAY")
	log.Info().Str("context", "init").Bool("value", env.NoRecording).Msg("DUCKSOUP_NO_RECORDING")
	log.Info().Str("context", "init").Str("value", fmt.Sprintf("%v", env.STUNServerURLS)).Msg("DUCKSOUP_STUN_SERVER_URLS")
}

func main() {
	// always build front (in watch mode or not, depending on env.Mode value, see front/build.go)
	frontbuild.Build()

	// run ducksoup only if not in FRONT_BUILD env.Mode
	if !cmdBuildMode {
		defer func() {
			if r := recover(); r != nil {
				log.Error().Str("context", "app").Err(fmt.Errorf("%v", r)).Msg("app_panicked")
			}
			log.Info().Str("context", "app").Msg("app_ended")
		}()

		// log initial state
		logState()

		// launch http (with websockets) server
		go server.Start()

		// launch TURN server
		go iceservers.StartTURN()
		defer iceservers.StopTURN()

		// start Glib main loop for GStreamer
		gst.StartMainLoop()
	}
}
