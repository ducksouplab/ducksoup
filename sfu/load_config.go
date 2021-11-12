package sfu

import (
	"os"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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
		log.Fatal().Err(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal().Err(err)
	}

	// log
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "20060102-150405.000"})
	log.Logger = log.With().Caller().Logger()
	log.Info().Msgf("[init] sfu config: %+v", config)
}
