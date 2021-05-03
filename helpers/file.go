// Package gst provides an easy API to create an appsink pipeline
package helpers

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

func ReadConfig(name string) string {
	var output string
	path := fmt.Sprintf("./config/%s.txt", name)
	f, err := os.Open(path)

	if err != nil {
		log.Fatal(err)
	}

	defer f.Close()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		output += scanner.Text()
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	return output
}

func EnsureDir(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		os.Mkdir(path, os.ModeDir)
	}
}
