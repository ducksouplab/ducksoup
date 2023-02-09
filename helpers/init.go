package helpers

import (
	"io"
	"os"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	timeFormat = "20060102-150405.000"
)

// used by file.go
var rootEnv string

func init() {
	// CAUTION: other init functions in "helpers" package may be called before this
	if os.Getenv("DS_ENV") == "DEV" {
		if err := godotenv.Load(".env"); err != nil {
			log.Fatal().Err(err)
		}
	}

	// used by file.go
	rootEnv = GetenvOr("DS_TEST_ROOT", ".") + "/"

	// zerolog defaults
	zerolog.TimeFieldFormat = timeFormat
	if Getenv("DS_ENV") == "DEV" {
		log.Logger = log.With().Caller().Logger()
	}

	// manage multi log output
	var writers []io.Writer
	// stdout writer
	if Getenv("DS_ENV") == "DEV" || Getenv("DS_LOG_STDOUT") == "true" {
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: timeFormat})
	}
	// file writer
	logFileEnv := Getenv("DS_LOG_FILE")
	if logFileEnv != "" {
		fileWriter, fileErr := os.OpenFile(logFileEnv, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if fileErr != nil {
			log.Error().Str("context", "init").Msg("error opening log file")
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
	levelEnv := Getenv("DS_LOG_LEVEL")
	zeroLevel := convertLevel(Getenv("DS_LOG_LEVEL"))
	zerolog.SetGlobalLevel(zeroLevel)
	log.Info().Str("context", "init").Str("level", levelEnv).Msg("logger_configured")
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
		return zerolog.DebugLevel
	}
}
