package env

import (
	"io"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// CAUTION relying on LogLevel package variable does not work (?) that's why we pass it as a parameter
func configureGlobalLogger(logLevel int) {
	// zerolog defaults
	zerolog.TimeFieldFormat = TimeFormat
	if Mode == "DEV" {
		log.Logger = log.With().Caller().Logger()
	}

	// manage multi log output
	var writers []io.Writer
	// stdout writer
	if Mode == "DEV" || LogStdout {
		writers = append(writers, zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: TimeFormat})
	}
	// file writer
	if LogFile != "" {
		fileWriter, fileErr := os.OpenFile(LogFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
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
	zeroLevel := convertLevel(logLevel)
	zerolog.SetGlobalLevel(zeroLevel)
}

func convertLevel(dsLevel int) zerolog.Level {
	switch dsLevel {
	case 0:
		return zerolog.FatalLevel
	case 1:
		return zerolog.ErrorLevel
	case 2:
		return zerolog.InfoLevel
	case 3:
		return zerolog.DebugLevel
	case 4:
		return zerolog.TraceLevel
	default:
		return zerolog.DebugLevel
	}
}
