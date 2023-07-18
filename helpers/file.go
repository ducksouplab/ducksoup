package helpers

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog/log"
)

var fileRoot string

func init() {
	fileRoot = "./"
	if strings.HasSuffix(os.Args[0], ".test") {
		fileRoot = "../"
	}
}

// Open file relatively to project
func Open(name string) (*os.File, error) {
	path := fmt.Sprintf(fileRoot+"%s", name)
	return os.Open(path)
}

func ReadFile(name string) string {
	var output string
	path := fmt.Sprintf(fileRoot+"%s", name)
	f, err := os.Open(path)

	if err != nil {
		log.Fatal().Err(err)
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		output += scanner.Text() + "\n"
	}

	if err := scanner.Err(); err != nil {
		log.Fatal().Err(err)
	}

	return output
}

func EnsureDir(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.MkdirAll(path, 0775)
	}
}
