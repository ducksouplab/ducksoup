package front

import (
	"github.com/ducksouplab/ducksoup/env"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/rs/zerolog/log"
)

var (
	developmentMode bool = false
	cmdBuildMode    bool = false
)

func init() {
	if env.Mode == "DEV" {
		developmentMode = true
	}
	if env.Mode == "BUILD_FRONT" {
		cmdBuildMode = true
	}
}

// API

func Build() {
	// only build in certain conditions (= not when launching ducksoup in production)
	if developmentMode || cmdBuildMode {
		logPlugin := api.Plugin{
			Name: "log",
			Setup: func(build api.PluginBuild) {
				build.OnEnd(func(result *api.BuildResult) (api.OnEndResult, error) {
					if len(result.Errors) > 0 {
						for _, msg := range result.Errors {
							log.Error().Str("context", "js_build").Msg(msg.Text)
						}
					} else {
						if len(result.Warnings) > 0 {
							for _, msg := range result.Warnings {
								log.Info().Str("context", "js_build").Msgf("%v", msg.Text)
							}
						} else {
							log.Info().Str("context", "js_build").Msg("build_success")
						}
					}
					return api.OnEndResult{}, nil
				})
			},
		}

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
			Outdir:  "front/static/assets/scripts",
			Plugins: []api.Plugin{logPlugin},
			Write:   true,
		}
		if developmentMode {
			ctx, err := api.Context(buildOptions)
			if err != nil {
				log.Fatal().Err(err)
			}

			watchErr := ctx.Watch(api.WatchOptions{})
			if watchErr != nil {
				log.Fatal().Err(watchErr)
			}
		} else {
			build := api.Build(buildOptions)

			if len(build.Errors) > 0 {
				log.Fatal().Str("context", "js_build").Msgf("%v", build.Errors[0].Text)
			}
		}
	}
}
