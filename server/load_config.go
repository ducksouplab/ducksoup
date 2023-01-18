package server

import (
	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v2"
)

type serverConfig struct {
	GenerateStats bool `yaml:"generateStats"`
}

var config serverConfig

func init() {
	f, err := helpers.Open("config/server.yml")
	if err != nil {
		log.Fatal().Err(err)
	}
	defer f.Close()

	decoder := yaml.NewDecoder(f)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal().Err(err)
	}
}
