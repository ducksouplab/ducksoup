package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ducksouplab/ducksoup/helpers"
	"github.com/ducksouplab/ducksoup/sfu"
	"github.com/ducksouplab/ducksoup/stats"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
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
			log.Info().Str("context", "peer").Str("origin", origin).Msg("websocket_upgraded")
			return helpers.Contains(allowedOrigins, origin)
		},
	}
)

func init() {
	// environment variables use
	envOriginsEnv := helpers.Getenv("DS_ORIGINS")
	if len(envOriginsEnv) > 0 {
		allowedOrigins = append(allowedOrigins, strings.Split(envOriginsEnv, ",")...)
	}
	if helpers.Getenv("DS_ENV") == "DEV" {
		allowedOrigins = append(allowedOrigins, "https://localhost:8000", "http://localhost:8000")
	}

	// web prefix, for instance "/path" if DuckSoup is reachable at https://host/path
	webPrefix = helpers.GetenvOr("DS_WEB_PREFIX", "")
	// basic Auth
	testLogin = helpers.GetenvOr("DS_TEST_LOGIN", "ducksoup")
	testPassword = helpers.GetenvOr("DS_TEST_PASSWORD", "ducksoup")
	statsLogin = helpers.GetenvOr("DS_STATS_LOGIN", "ducksoup")
	statsPassword = helpers.GetenvOr("DS_STATS_PASSWORD", "ducksoup")

	// log
	log.Info().Str("context", "init").Str("origins", fmt.Sprintf("%v", allowedOrigins)).Msg("websocket_origins_allowed")
}

// handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Str("context", "peer").Err(err).Msg("upgrade_websocket_failed")
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

func notFound(w http.ResponseWriter, r *http.Request) {
	log.Info().Str("context", "server").Str("URL", r.URL.String()).Msg("not_found")
}

// API

func ListenAndServe() {
	// parse the flags passed to program
	flag.Parse()

	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(notFound)
	// websocket handler
	router.HandleFunc(webPrefix+"/ws", websocketHandler)

	// assets without basic auth
	router.PathPrefix(webPrefix + "/assets/").Handler(http.StripPrefix(webPrefix+"/assets/", http.FileServer(http.Dir("./front/static/assets/"))))

	// test pages with basic auth
	testRouter := router.PathPrefix(webPrefix + "/test").Subrouter()
	testRouter.Use(basicAuthWith(testLogin, testPassword))
	testRouter.PathPrefix("/mirror/").Handler(http.StripPrefix(webPrefix+"/test/mirror/", http.FileServer(http.Dir("./front/static/pages/test/mirror/"))))
	testRouter.PathPrefix("/room/").Handler(http.StripPrefix(webPrefix+"/test/room/", http.FileServer(http.Dir("./front/static/pages/test/room/"))))
	testRouter.PathPrefix("/play/").Handler(http.StripPrefix(webPrefix+"/test/play/", http.FileServer(http.Dir("./front/static/pages/test/play/"))))

	// stats pages with basic auth
	if config.GenerateStats {
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
		log.Info().Str("context", "init").Str("port", port).Msg("https_server_started")
		log.Fatal().Err(server.ListenAndServeTLS(*cert, *key)) // blocking
	} else {
		log.Info().Str("context", "init").Str("port", port).Msg("http_server_started")
		log.Fatal().Err(server.ListenAndServe()) // blocking
	}
}
