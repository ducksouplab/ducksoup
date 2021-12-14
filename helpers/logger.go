package helpers

import (
	"io"
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
	if os.Getenv("DS_ENV") == "DEV" {
		log.Logger = log.With().Caller().Logger()
	}

	// manage multi log output
	var writers []io.Writer
	// stdout writer
	if os.Getenv("DS_ENV") == "DEV" || os.Getenv("DS_LOG_STDOUT") == "true" {
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: timeFormat})
	}
	// file writer
	logFile := os.Getenv("DS_LOG_FILE")
	if logFile != "" {
		fileWriter, fileErr := os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if fileErr != nil {
			log.Error().Msg("error opening log file")
		} else {
			writers = append(writers, fileWriter)
		}
	}
	// set writers
	if len(writers) == 1 {
		log.Logger = log.Output(writers[0])
	} else if len(writers) > 1 {
		multi := zerolog.MultiLevelWriter(writers...)
		log.Logger = log.Output(multi)
	}
	// set level
	level := convertLevel(os.Getenv("DS_LOG_LEVEL"))
	zerolog.SetGlobalLevel(level)
	log.Info().Msgf("[init] logger configured with level: %v", level)
}

func convertLevel(dsLevel string) zerolog.Level {
	switch dsLevel {
	case "0":
		return zerolog.Disabled
	case "1":
		return zerolog.ErrorLevel
	case "2":
		return zerolog.InfoLevel
	case "3":
		return zerolog.DebugLevel
	case "4":
		return zerolog.TraceLevel
	default:
		return zerolog.InfoLevel
	}
}