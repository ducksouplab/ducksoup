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

type frontendConfig struct {
	Version string
}

var SFU SFUConfig
var FrontendVersion string

func init() {
	// SFU
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

	// Front-end
	f, err = helpers.Open("config/front.yml")
	if err != nil {
		log.Fatal().Err(err)
	}
	defer f.Close()

	var frontend frontendConfig
	decoder = yaml.NewDecoder(f)
	err = decoder.Decode(&frontend)
	if err != nil {
		log.Fatal().Err(err)
	}
	FrontendVersion = frontend.Version

	// log
	log.Info().Str("context", "init").Str("config", fmt.Sprintf("%+v", SFU)).Msg("sfu_config_loaded")
	log.Info().Str("context", "init").Str("value", FrontendVersion).Msg("frontend_version")
}
