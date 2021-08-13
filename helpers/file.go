package helpers

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

var root string

func init() {
	root = Getenv("DS_TEST_ROOT", ".") + "/"
}

// Open file relatively to project
func Open(name string) (*os.File, error) {
	path := fmt.Sprintf(root+"%s", name)
	return os.Open(path)
}

func ReadFile(name string) string {
	var output string
	path := fmt.Sprintf(root+"%s", name)
	f, err := os.Open(path)

	if err != nil {
		log.Fatal("[fatal] ", err)
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		output += scanner.Text() + "\n"
	}

	if err := scanner.Err(); err != nil {
		log.Fatal("[fatal] ", err)
	}

	return output
}

func EnsureDir(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, 0775)
	}
}
