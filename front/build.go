package front

import (
	"log"
	"os"

	"github.com/evanw/esbuild/pkg/api"
)

var (
	developmentMode bool = false
)

func init() {
	if os.Getenv("APP_ENV") == "DEV" {
		developmentMode = true
	}
}

func Build() {
	buildOptions := api.BuildOptions{
		EntryPoints:       []string{"front/src/1on1/app.js", "front/src/test/app.js", "front/src/embed/app.js"},
		Bundle:            true,
		MinifyWhitespace:  true,
		MinifyIdentifiers: true,
		MinifySyntax:      true,
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
					log.Printf("watch build failed: %d errors\n", len(result.Errors))
					log.Println(result.Errors)
				} else {
					if len(result.Warnings) > 0 {
						log.Printf("watch build succeeded: %d warnings\n", len(result.Warnings))
						log.Println(result.Warnings)
					} else {
						log.Println("watch build succeeded")
					}
				}
			},
		}
	}
	build := api.Build(buildOptions)

	if len(build.Errors) > 0 {
		log.Fatal(build.Errors)
	}
}
