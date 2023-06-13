package config

import (
	"fmt"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type SFUConfig struct {
	Common struct {
		MTU                  int `yaml:"mtu"`
		EncoderControlPeriod int `yaml:"encoderControlPeriod"`
		TWCCInterval         int `yaml:"twccInterval"`
	}
	Audio SFUStream
	Video SFUStream
}

type SFUStream struct {
	DefaultBitrate int `yaml:"defaultBitrate"`
	MinBitrate     int `yaml:"minBitrate"`
	MaxBitrate     int `yaml:"maxBitrate"`
}

var SFU SFUConfig

func init() {
	f, err := helpers.Open("config/sfu.yml")
	if err != nil {
		log.Fatal().Err(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&SFU)
	if err != nil {
		log.Fatal().Err(err)
	}

	// log
	log.Info().Str("context", "init").Str("config", fmt.Sprintf("%+v", SFU)).Msg("sfu_config_loaded")
}
