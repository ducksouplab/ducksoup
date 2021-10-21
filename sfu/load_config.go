package sfu

import (
	"log"

	"github.com/creamlab/ducksoup/helpers"
	"gopkg.in/yaml.v2"
)

type sfuConfig struct {
	Audio sfuStream
	Video sfuStream
}

type sfuStream struct {
	DefaultBitrate uint64
	MinBitrate     uint64
	MaxBitrate     uint64
}

var config sfuConfig

func init() {

	f, err := helpers.Open("config/sfu.yml")
	if err != nil {
		log.Fatal("[fatal] ", err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal("[fatal] ", err)
	}
}
