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
	DefaultBitrate uint64 `yaml:"defaultBitrate"`
	MinBitrate     uint64 `yaml:"minBitrate"`
	MaxBitrate     uint64 `yaml:"maxBitrate"`
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

	// log
	log.SetFlags(log.Lmicroseconds)
	log.Printf("[info] [init] sfu config: %+v\n", config)
}
