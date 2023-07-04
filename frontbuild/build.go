package frontbuild

import (
	"github.com/ducksouplab/ducksoup/config"
	"github.com/ducksouplab/ducksoup/env"
	"github.com/evanw/esbuild/pkg/api"
	"github.com/rs/zerolog/log"
)

var (
	devMode      bool = false
	cmdBuildMode bool = false
)

func init() {
	if env.Mode == "DEV" {
		devMode = true
	}
	if env.Mode == "FRONT_BUILD" {
		cmdBuildMode = true
	}
}

func generateBuildOptions(versioned bool) api.BuildOptions {
	entryNames := config.FrontendVersion + "/[ext]/[name]"
	outdir := "front/static/assets"
	locationLog := config.FrontendVersion
	if !versioned {
		entryNames = "[ext]/[name]"
		outdir = "front/static/assets/latest"
		locationLog = "latest"
	}

	includesPlugin := api.Plugin{
		Name: "Update includes",
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
					} else { // success path
						log.Info().Str("context", "js_build").Str("location", locationLog).Msg("build_success")
						updateTemplates()
						cleanUpAssets()
					}
				}
				return api.OnEndResult{}, nil
			})
		},
	}
	return api.BuildOptions{
		EntryPoints: []string{
			"front/src/js/lib/ducksoup.js",
			"front/src/js/test/play/play.jsx",
			"front/src/js/test/mirror/mirror.js",
			"front/src/js/stats/stats.js",
			"front/src/css/mirror.css",
			"front/src/css/play.css",
		},
		EntryNames:        entryNames,
		Bundle:            true,
		MinifyWhitespace:  !devMode,
		MinifyIdentifiers: !devMode,
		MinifySyntax:      !devMode,
		Engines: []api.Engine{
			{api.EngineChrome, "64"},
			{api.EngineFirefox, "53"},
			{api.EngineSafari, "11"},
			{api.EngineEdge, "79"},
		},
		Outdir:  outdir,
		Plugins: []api.Plugin{includesPlugin},
		Write:   true,
	}
}

// API

func Build() {
	// only build in certain conditions (= not when launching ducksoup in production)
	if devMode || cmdBuildMode {

		versionedBuildOptions := generateBuildOptions(true)
		latestBuildOptions := generateBuildOptions(false)
		if devMode {
			ctx, err := api.Context(versionedBuildOptions)
			if err != nil {
				log.Fatal().Err(err)
			} else if watchErr := ctx.Watch(api.WatchOptions{}); watchErr != nil {
				log.Fatal().Err(watchErr)
			}
			ctx, err = api.Context(latestBuildOptions)
			if err != nil {
				log.Fatal().Err(err)
			} else if watchErr := ctx.Watch(api.WatchOptions{}); watchErr != nil {
				log.Fatal().Err(watchErr)
			}
		} else {
			api.Build(versionedBuildOptions)
			api.Build(latestBuildOptions)
		}
	}
}
