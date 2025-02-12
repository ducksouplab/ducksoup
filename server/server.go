package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ducksouplab/ducksoup/env"
	"github.com/ducksouplab/ducksoup/sfu"
	"github.com/ducksouplab/ducksoup/stats"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

var (
	cert     = flag.String("cert", "", "cert file")
	key      = flag.String("key", "", "key file")
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return slices.Contains(env.AllowedWSOrigins, origin)
		},
	}
)

// handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Str("context", "peer").Err(err).Msg("upgrade_websocket_failed")
		return
	}
	origin := r.Header.Get("Origin")
	href, _ := url.QueryUnescape(r.FormValue("href"))
	log.Info().Str("context", "peer").Str("origin", origin).Str("href", href).Msg("websocket_upgraded")

	if r.FormValue("type") == "stats" {
		// special path: ws for stats
		if config.GenerateStats { // protect endpoint according to server setting
			stats.RunStatsServer(unsafeConn) // blocking
		}
	} else {
		// main path: ws for peer signaling
		sfu.RunPeerServer(origin, unsafeConn) // blocking
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

// ################ Saving noise and volume level from test pages ########################//
// Request payload structure
type AudioDataPayload struct {
	Namespace   string    `json:"namespace"`
	Interaction string    `json:"interaction"`
	Data        AudioData `json:"data"`
}

// Write struct
type AudioData struct {
	NoiseLevels  float64 `json:"noiseLevels"`
	VolumeLevels float64 `json:"volumeLevels"`
	Passed       bool    `json:"passed"`
	Timestamp    string  `json:"timestamp"`
}

func SaveAudioTestResult(w http.ResponseWriter, r *http.Request) {
	// Ensure POST request
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var payload AudioDataPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Format data as JSON for easy parsing later
	formattedData, err := json.MarshalIndent(payload.Data, "", "  ")
	if err != nil {
		http.Error(w, "Data formatting failed", http.StatusInternalServerError)
		return
	}

	// Create directory path and ensure it exists
	dirPath := filepath.Join("data", payload.Namespace, payload.Interaction)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		http.Error(w, "Storage error", http.StatusInternalServerError)
		return
	}

	// Write data to file
	filePath := filepath.Join(dirPath, "audio_test_results.json")
	if err := os.WriteFile(filePath, append(formattedData, '\n'), 0644); err != nil {
		http.Error(w, "Write error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

//################ Saving noise and volume level from test pages ########################//

// API

func Start() {
	webPrefix := env.WebPrefix
	// parse the flags passed to program
	flag.Parse()

	router := mux.NewRouter()
	router.NotFoundHandler = http.HandlerFunc(notFound)
	// websocket handler
	router.HandleFunc(webPrefix+"/ws", websocketHandler)

	// handler for audio_test
	router.HandleFunc("/POST_audio_test", SaveAudioTestResult)

	// assets without basic auth
	router.PathPrefix(webPrefix + "/assets/").Handler(http.StripPrefix(webPrefix+"/assets/", http.FileServer(http.Dir("./front/static/assets/"))))
	router.PathPrefix(webPrefix + "/config/").Handler(http.StripPrefix(webPrefix+"/config/", http.FileServer(http.Dir("./front/static/config/"))))

	directRouter := router.PathPrefix(webPrefix + "/test").Subrouter()
	directRouter.PathPrefix("/direct/").Handler(http.StripPrefix(webPrefix+"/test/direct/", http.FileServer(http.Dir("./front/static/pages/test/direct/"))))
	directRouter.PathPrefix("/audio_direct/").Handler(http.StripPrefix(webPrefix+"/test/audio_direct/", http.FileServer(http.Dir("./front/static/pages/test/audio_direct/"))))

	testRouter := router.PathPrefix(webPrefix + "/test").Subrouter()

	// test pages with basic auth
	testRouter.Use(basicAuthWith(env.TestLogin, env.TestPassword))
	testRouter.PathPrefix("/ice/").Handler(http.StripPrefix(webPrefix+"/test/ice/", http.FileServer(http.Dir("./front/static/pages/test/ice/"))))
	testRouter.PathPrefix("/mirror/").Handler(http.StripPrefix(webPrefix+"/test/mirror/", http.FileServer(http.Dir("./front/static/pages/test/mirror/"))))
	testRouter.PathPrefix("/interaction/").Handler(http.StripPrefix(webPrefix+"/test/interaction/", http.FileServer(http.Dir("./front/static/pages/test/interaction/"))))
	testRouter.PathPrefix("/play/").Handler(http.StripPrefix(webPrefix+"/test/play/", http.FileServer(http.Dir("./front/static/pages/test/play/"))))

	// stats pages with basic auth
	if config.GenerateStats {
		statsRouter := router.PathPrefix(webPrefix + "/stats").Subrouter()
		statsRouter.Use(basicAuthWith(env.TestLogin, env.TestPassword))
		statsRouter.PathPrefix("/").Handler(http.StripPrefix(webPrefix+"/stats/", http.FileServer(http.Dir("./front/static/pages/stats/"))))
	}

	server := &http.Server{
		Handler:      router,
		Addr:         ":" + env.Port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	// start server
	if *key == "" || *cert == "" { // http
		log.Info().Str("context", "init").Str("port", env.Port).Msg("http_server_started")
		log.Fatal().Err(server.ListenAndServe()) // blocking
	} else { // https
		log.Info().Str("context", "init").Str("port", env.Port).Msg("https_server_started")
		log.Fatal().Err(server.ListenAndServeTLS(*cert, *key)) // blocking
	}
}
