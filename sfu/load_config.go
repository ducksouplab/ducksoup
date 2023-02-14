package sfu

import (
	"fmt"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type sfuConfig struct {
	Common struct {
		MTU uint64 `yaml:"mtu"`
	}
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
		log.Fatal().Err(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal().Err(err)
	}

	// log
	log.Info().Str("context", "init").Str("config", fmt.Sprintf("%+v", config)).Msg("sfu_config_loaded")
}
