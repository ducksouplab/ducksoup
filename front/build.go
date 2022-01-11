package front

import (
	"os"

	_ "github.com/creamlab/ducksoup/helpers" // rely on helpers logger init side-effect
	"github.com/evanw/esbuild/pkg/api"
	"github.com/rs/zerolog/log"
)

var (
	developmentMode bool = false
	cmdBuildMode    bool = false
)

func init() {
	if os.Getenv("DS_ENV") == "DEV" {
		developmentMode = true
	}
	if os.Getenv("DS_ENV") == "BUILD_FRONT" {
		cmdBuildMode = true
	}
}

// API

func Build() {
	// only build in certain conditions (= not when launching ducksoup in production)
	if developmentMode || cmdBuildMode {
		buildOptions := api.BuildOptions{
			EntryPoints:       []string{"front/src/lib/ducksoup.js", "front/src/test/play/app.jsx", "front/src/test/mirror/app.js", "front/src/stats/app.js"},
			Bundle:            true,
			MinifyWhitespace:  !developmentMode,
			MinifyIdentifiers: !developmentMode,
			MinifySyntax:      !developmentMode,
			Engines: []api.Engine{
				{api.EngineChrome, "64"},
				{api.EngineFirefox, "53"},
				{api.EngineSafari, "11"},
				{api.EngineEdge, "79"},
			},
			Outdir: "front/static/assets/scripts",
			Write:  true,
		}
		if developmentMode {
			buildOptions.Watch = &api.WatchMode{
				OnRebuild: func(result api.BuildResult) {
					if len(result.Errors) > 0 {
						for _, msg := range result.Errors {
							log.Error().Str("context", "js-build").Msg(msg.Text)
						}
					} else {
						if len(result.Warnings) > 0 {
							for _, msg := range result.Warnings {
								log.Info().Str("context", "js-build").Msgf("%v", msg.Text)
							}
						} else {
							log.Info().Str("context", "js-build")
						}
					}
				},
			}
		}
		build := api.Build(buildOptions)

		if len(build.Errors) > 0 {
			log.Fatal().Str("context", "js-build").Msgf("%v", build.Errors[0].Text)
		}
	}
}
