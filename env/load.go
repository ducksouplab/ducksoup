package env

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog/log"
)

const (
	TimeFormat = "20060102-150405.000"
)

var ForceOverlay, GCC, GSTTracking, GeneratePlots, GenerateTWCC, LogStdout, NoRecording, NVCodec, NVCuda bool
var LogLevel int
var LogFile, Mode, Port, PublicIP, TestLogin, TestPassword, WebPrefix string
var AllowedWSOrigins, ICEServers []string

func getenvOr(key, fallback string) string {
	value := os.Getenv(key)
	if len(value) == 0 {
		return fallback
	}
	return value
}

func init() {
	Mode = getenvOr("DUCKSOUP_MODE", "PROD")
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
	if strings.ToLower(os.Getenv("DUCKSOUP_GENERATE_PLOTS")) == "true" {
		GeneratePlots = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_GENERATE_TWCC")) == "true" {
		GenerateTWCC = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_LOG_STDOUT")) == "true" {
		LogStdout = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_NO_RECORDING")) == "true" {
		NoRecording = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_NVCODEC")) == "true" {
		NVCodec = true
	}
	if strings.ToLower(os.Getenv("DUCKSOUP_NVCUDA")) == "true" {
		NVCuda = true
	}

	// uints
	var err error
	LogLevel, err = strconv.Atoi(os.Getenv("DUCKSOUP_LOG_LEVEL"))

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

	// other global configuration
	configureGlobalLogger(LogLevel)
}
