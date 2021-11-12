package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"flag"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/creamlab/ducksoup/sfu"
	"github.com/creamlab/ducksoup/stats"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	port           string
	allowedOrigins = []string{}
	webPrefix      string
	testLogin      string
	testPassword   string
	statsLogin     string
	statsPassword  string
	cert           = flag.String("cert", "", "cert file")
	key            = flag.String("key", "", "key file")
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			log.Info().Msgf("[server] ws upgrade from origin: ", origin)
			return helpers.Contains(allowedOrigins, origin)
		},
	}
)

func init() {
	// environment variables use
	envOrigins := os.Getenv("DS_ORIGINS")
	if len(envOrigins) > 0 {
		allowedOrigins = append(allowedOrigins, strings.Split(envOrigins, ",")...)
	}
	if os.Getenv("DS_ENV") == "DEV" {
		allowedOrigins = append(allowedOrigins, "https://localhost:8080", "https://localhost:8000", "http://localhost:8000")
	}

	// web prefix, for instance "/path" if DuckSoup is reachable at https://host/path
	webPrefix = helpers.Getenv("DS_WEB_PREFIX", "")
	// basict Auth
	testLogin = helpers.Getenv("DS_TEST_LOGIN", "ducksoup")
	testPassword = helpers.Getenv("DS_TEST_PASSWORD", "ducksoup")
	statsLogin = helpers.Getenv("DS_STATS_LOGIN", "ducksoup")
	statsPassword = helpers.Getenv("DS_STATS_PASSWORD", "ducksoup")

	// log
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMicro
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "20060102-150405.000"})
	log.Logger = log.With().Caller().Logger()
	log.Info().Msgf("[server] allowed ws origins: %v", allowedOrigins)
}

// handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("[error] [server] [ws] can't upgrade:", err)
		return
	}

	if r.FormValue("type") == "stats" {
		// special path: ws for stats
		if config.GenerateStats { // protect endpoint according to server setting
			stats.RunStatsServer(unsafeConn) // blocking
		}
	} else {
		// main path: ws for peer signaling
		sfu.RunPeerServer(r.Header.Get("Origin"), unsafeConn) // blocking
	}
}

func basicAuthWith(refLogin, refPassword string) mux.MiddlewareFunc {
	// source https://www.alexedwards.net/blog/basic-authentication-in-go
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			login, password, ok := r.BasicAuth()
			if ok {
				// Calculate SHA-256 hashes for the provided and expected usernames and passwords.
				loginHash := sha256.Sum256([]byte(login))
				passwordHash := sha256.Sum256([]byte(password))
				expectedLoginHash := sha256.Sum256([]byte(refLogin))
				expectedPasswordHash := sha256.Sum256([]byte(refPassword))

				loginMatch := (subtle.ConstantTimeCompare(loginHash[:], expectedLoginHash[:]) == 1)
				passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1)

				if loginMatch && passwordMatch {
					next.ServeHTTP(w, r)
					return
				}
			}

			w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		})
	}
}

// API

func ListenAndServe() {
	// parse the flags passed to program
	flag.Parse()

	router := mux.NewRouter()
	// websocket handler
	router.HandleFunc(webPrefix+"/ws", websocketHandler)

	// assets (js & css) without basic auth
	router.PathPrefix(webPrefix + "/scripts/").Handler(http.StripPrefix(webPrefix+"/scripts/", http.FileServer(http.Dir("./front/static/assets/scripts/"))))
	router.PathPrefix(webPrefix + "/styles/").Handler(http.StripPrefix(webPrefix+"/styles/", http.FileServer(http.Dir("./front/static/assets/styles/"))))

	// test pages with basic auth
	testRouter := router.PathPrefix(webPrefix + "/test").Subrouter()
	testRouter.Use(basicAuthWith(testLogin, testPassword))
	testRouter.PathPrefix("/mirror/").Handler(http.StripPrefix(webPrefix+"/test/mirror/", http.FileServer(http.Dir("./front/static/pages/test/mirror/"))))
	testRouter.PathPrefix("/room/").Handler(http.StripPrefix(webPrefix+"/test/room/", http.FileServer(http.Dir("./front/static/pages/test/room/"))))

	if config.GenerateStats {
		// stats pages with basic auth
		statsRouter := router.PathPrefix(webPrefix + "/stats").Subrouter()
		statsRouter.Use(basicAuthWith(statsLogin, statsPassword))
		statsRouter.PathPrefix("/").Handler(http.StripPrefix(webPrefix+"/stats/", http.FileServer(http.Dir("./front/static/pages/stats/"))))
	}

	// port
	port = os.Getenv("DS_PORT")
	if len(port) < 2 {
		port = "8000"
	}

	server := &http.Server{
		Handler:      router,
		Addr:         ":" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	// start HTTP server
	if *key != "" && *cert != "" {
		log.Info().Msgf("[server] https listening on port " + port)
		log.Fatal().Err(server.ListenAndServeTLS(*cert, *key)) // blocking
	} else {
		log.Info().Msgf("[server] http listening on port " + port)
		log.Fatal().Err(server.ListenAndServe()) // blocking
	}
}
