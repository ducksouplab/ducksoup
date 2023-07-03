package frontbuild

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ducksouplab/ducksoup/config"
)

var jsLineRegex, jsUpdateRegex, cssLineRegex, cssUpdateRegex *regexp.Regexp

func init() {
	jsLineRegex = regexp.MustCompile("script.*/assets/(.*)/js.*js")
	jsUpdateRegex = regexp.MustCompile("assets/.*?/js")
	cssLineRegex = regexp.MustCompile("link.*/assets/(.*)/css.*css")
	cssUpdateRegex = regexp.MustCompile("assets/.*?/css")
}

func cleanUpAssets() {
	path := "front/static/assets/"
	infos, err := os.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}

	for _, info := range infos {
		if info.IsDir() && info.Name() != config.FrontendVersion {
			os.RemoveAll(path + info.Name())
		}
	}
}

func updateTemplates() {
	filepath.Walk("front/static/pages/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err)
			return err
		}
		if !info.IsDir() {
			replaceIncludes(path)
		}
		return nil
	})
}

func replaceIncludes(path string) {
	input, err := os.ReadFile(path)
	if err != nil {
		log.Fatalln(err)
	}

	lines := strings.Split(string(input), "\n")

	var js, css bool
	for i, line := range lines {
		if jsLineRegex.MatchString(line) {
			lines[i] = jsUpdateRegex.ReplaceAllString(line, "assets/"+config.FrontendVersion+"/js")
			js = true
		} else if cssLineRegex.MatchString(line) {
			lines[i] = cssUpdateRegex.ReplaceAllString(line, "assets/"+config.FrontendVersion+"/css")
			css = true
		}
	}
	// log once
	if js {
		log.Printf("[Template] %v CSS prefixed with version %v\n", path, config.FrontendVersion)
	}
	if css {
		log.Printf("[Template] %v  JS prefixed with version %v\n", path, config.FrontendVersion)
	}

	output := strings.Join(lines, "\n")
	err = os.WriteFile(path, []byte(output), 0644)
	if err != nil {
		log.Fatalln(err)
	}
}
