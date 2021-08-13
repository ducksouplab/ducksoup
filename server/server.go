package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/creamlab/ducksoup/helpers"
	"github.com/creamlab/ducksoup/sfu"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var (
	port           string
	allowedOrigins = []string{}
	testLogin      string
	testPassword   string
	cert           = flag.String("cert", "", "cert file")
	key            = flag.String("key", "", "key file")
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			log.Println("[ws] upgrade from origin: ", origin)
			return helpers.Contains(allowedOrigins, origin)
		},
	}
)

func init() {
	envOrigins := os.Getenv("DS_ORIGINS")
	if len(envOrigins) > 0 {
		allowedOrigins = append(allowedOrigins, strings.Split(envOrigins, ",")...)
	}
	if os.Getenv("DS_ENV") == "DEV" {
		allowedOrigins = append(allowedOrigins, "https://localhost:8080", "https://localhost:8000", "http://localhost:8000")
	}
	// basict Auth
	testLogin = helpers.Getenv("DS_TEST_LOGIN", "ducksoup")
	testPassword = helpers.Getenv("DS_TEST_PASSWORD", "ducksoup")
}

// handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	sfu.RunPeerServer(r.Header.Get("Origin"), unsafeConn) // blocking
}

// source https://www.alexedwards.net/blog/basic-authentication-in-go
func basicAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok {
			// Calculate SHA-256 hashes for the provided and expected usernames and passwords.
			usernameHash := sha256.Sum256([]byte(username))
			passwordHash := sha256.Sum256([]byte(password))
			expectedUsernameHash := sha256.Sum256([]byte(testLogin))
			expectedPasswordHash := sha256.Sum256([]byte(testPassword))

			usernameMatch := (subtle.ConstantTimeCompare(usernameHash[:], expectedUsernameHash[:]) == 1)
			passwordMatch := (subtle.ConstantTimeCompare(passwordHash[:], expectedPasswordHash[:]) == 1)

			if usernameMatch && passwordMatch {
				next.ServeHTTP(w, r)
				return
			}
		}

		w.Header().Set("WWW-Authenticate", `Basic realm="restricted", charset="UTF-8"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

// API

func ListenAndServe() {
	// parse the flags passed to program
	flag.Parse()

	// init logging
	log.SetFlags(log.Lmicroseconds)
	log.SetOutput(os.Stdout)

	router := mux.NewRouter()

	// js & css and html without basic auth
	router.PathPrefix("/scripts/").Handler(http.StripPrefix("/scripts/", http.FileServer(http.Dir("./front/static/assets/scripts/"))))
	router.PathPrefix("/styles/").Handler(http.StripPrefix("/styles/", http.FileServer(http.Dir("./front/static/assets/styles/"))))
	// html with basic auth
	testRouter := router.PathPrefix("/test").Subrouter()
	testRouter.Use(basicAuth)
	testRouter.PathPrefix("/mirror/").Handler(http.StripPrefix("/test/mirror/", http.FileServer(http.Dir("./front/static/pages/test/mirror/"))))
	testRouter.PathPrefix("/room/").Handler(http.StripPrefix("/test/room/", http.FileServer(http.Dir("./front/static/pages/test/room/"))))

	// websocket handler
	router.HandleFunc("/ws", websocketHandler)

	// port
	port = ":" + os.Getenv("DS_PORT")
	if len(port) < 2 {
		port = ":8000"
	}

	server := &http.Server{
		Handler:      router,
		Addr:         port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	// start HTTP server
	if *key != "" && *cert != "" {
		log.Println("[main] https listening on " + port)
		log.Fatal(server.ListenAndServeTLS(*cert, *key)) // blocking
	} else {
		log.Println("[main] http listening on " + port)
		log.Fatal(server.ListenAndServe()) // blocking
	}
}
