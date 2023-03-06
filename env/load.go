package env

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

var ForceOverlay, GCC, GSTTracking, GenerateTWCC, LogStdout, NVCodec bool
var LogLevel int
var LogFile, Mode, Port, ProjectRoot, PublicIP, TestLogin, TestPassword, WebPrefix string
var AllowedWSOrigins, ICEServers []string

func getenvOr(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func init() {
	Mode = os.Getenv("DUCKSOUP_MODE")
	// CAUTION: other init functions in "helpers" package may be called before this
	if Mode == "DEV" {
		if err := godotenv.Load(".env"); err != nil {
			log.Fatal().Err(err)
		}
	}
	// bools
	if strings.ToLower(os.Getenv("DUCKSOUP_FORCE_OVERLAY")) == "true" {
		ForceOverlay = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_GCC")) == "true" {
		GCC = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_GST_TRACKING")) == "true" {
		GSTTracking = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_GENERATE_TWCC")) == "true" {
		GenerateTWCC = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_LOG_STDOUT")) == "true" {
		LogStdout = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_NVCODEC")) == "true" {
		NVCodec = true
	}
	// uints
	LogLevel, err := strconv.Atoi(os.Getenv("DUCKSOUP_LOG_LEVEL"))

	if err != nil {
		LogLevel = 3
	}

	// strings
	LogFile = os.Getenv("DUCKSOUP_LOG_FILE")
	Port = os.Getenv("DUCKSOUP_PORT")
	if len(Port) < 2 {
		Port = "8100"
	}
	PublicIP = os.Getenv("DUCKSOUP_PUBLIC_IP")
	ProjectRoot = getenvOr("DUCKSOUP_PROJECT_ROOT", ".") + "/"
	// for instance "/path" if DuckSoup is reachable at https://host/path
	WebPrefix = getenvOr("DUCKSOUP_WEB_PREFIX", "")
	// basic Auth
	TestLogin = getenvOr("DUCKSOUP_TEST_LOGIN", "ducksoup")
	TestPassword = getenvOr("DUCKSOUP_TEST_PASSWORD", "ducksoup")
	// origins
	originsUnsplit := os.Getenv("DUCKSOUP_ALLOWED_WS_ORIGINS")
	if len(originsUnsplit) > 0 {
		AllowedWSOrigins = append(AllowedWSOrigins, strings.Split(originsUnsplit, ",")...)
	}
	if Mode == "DEV" {
		AllowedWSOrigins = append(AllowedWSOrigins, "http://localhost:"+Port, "http://localhost:8180")
	}
	// ICE servers
	iceServersUnsplit := os.Getenv("DUCKSOUP_ICE_SERVERS")
	if iceServersUnsplit == "false" {
		ICEServers = []string{}
	} else if len(iceServersUnsplit) > 0 {
		ICEServers = append(ICEServers, strings.Split(iceServersUnsplit, ",")...)
	} else { // default
		ICEServers = []string{"stun:stun.l.google.com:19302"}
	}

	// log
	log.Info().Str("context", "env").Str("value", Mode).Msg("DUCKSOUP_MODE")
	log.Info().Str("context", "env").Str("value", WebPrefix).Msg("DUCKSOUP_WEB_PREFIX")
	log.Info().Str("context", "env").Bool("value", ForceOverlay).Msg("DUCKSOUP_FORCE_OVERLAY")
	log.Info().Str("context", "env").Bool("value", GCC).Msg("DUCKSOUP_GCC")
	log.Info().Str("context", "env").Bool("value", GSTTracking).Msg("DUCKSOUP_GST_TRACKING")
	log.Info().Str("context", "env").Bool("value", GenerateTWCC).Msg("DUCKSOUP_GENERATE_TWCC")
	log.Info().Str("context", "env").Bool("value", NVCodec).Msg("DUCKSOUP_NVCODEC")
	log.Info().Str("context", "env").Int("value", LogLevel).Msg("DUCKSOUP_LOG_LEVEL")
	log.Info().Str("context", "env").Str("value", fmt.Sprintf("%v", AllowedWSOrigins)).Msg("DUCKSOUP_ALLOWED_WS_ORIGINS")
	log.Info().Str("context", "env").Str("value", fmt.Sprintf("%v", ICEServers)).Msg("DUCKSOUP_ICE_SERVERS")

	// other global configuration
	configureLogger(LogLevel)
}
