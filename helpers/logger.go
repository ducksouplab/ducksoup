package helpers

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	timeFormat = "20060102-150405.000"
)

func init() {
	// zerolog defaults
	zerolog.TimeFieldFormat = timeFormat
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: timeFormat})
	if os.Getenv("DS_ENV") == "DEV" {
		log.Logger = log.With().Caller().Logger()
	}
	// change logger output if a log file is provided
	logFile := os.Getenv("DS_LOG_FILE")
	if logFile != "" {
		output, fileErr := os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if fileErr != nil {
			log.Error().Msg("error opening log file -> choosing Stdout as log output")
		} else {
			log.Logger = log.Output(output)
		}
	}
	log.Info().Msg("logger configured")
}
