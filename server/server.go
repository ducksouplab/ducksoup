package server

import (
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
	allowedOrigins = []string{}
	addr           = flag.String("addr", ":8080", "http service address")
	cert           = flag.String("cert", "", "cert file")
	key            = flag.String("key", "", "key file")
	upgrader       = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			log.Println(origin)
			return helpers.Contains(allowedOrigins, origin)
		},
	}
)

func init() {
	envOrigins := os.Getenv("ORIGINS")
	if len(envOrigins) > 0 {
		allowedOrigins = append(allowedOrigins, strings.Split(envOrigins, ",")...)
	}
	if os.Getenv("APP_ENV") == "DEV" {
		allowedOrigins = append(allowedOrigins, "https://localhost:8000", "https://localhost:8080", "http://localhost:8080")
	}
}

// handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	sfu.RunPeerServer(unsafeConn) // blocking
}

// API

func ListenAndServe() {
	// parse the flags passed to program
	flag.Parse()

	// init logging
	log.SetFlags(log.Lmicroseconds)
	log.SetOutput(os.Stdout)

	router := mux.NewRouter()

	// js & css
	router.PathPrefix("/scripts/").Handler(http.StripPrefix("/scripts/", http.FileServer(http.Dir("./front/static/assets/scripts/"))))
	router.PathPrefix("/styles/").Handler(http.StripPrefix("/styles/", http.FileServer(http.Dir("./front/static/assets/styles/"))))
	// html
	router.PathPrefix("/embed/").Handler(http.StripPrefix("/embed/", http.FileServer(http.Dir("./front/static/pages/embed/"))))
	router.PathPrefix("/test_embed/").Handler(http.StripPrefix("/test_embed/", http.FileServer(http.Dir("./front/static/pages/test_embed/"))))
	router.PathPrefix("/test_standalone/").Handler(http.StripPrefix("/test_standalone/", http.FileServer(http.Dir("./front/static/pages/test_standalone/"))))

	// websocket handler
	router.HandleFunc("/ws", websocketHandler)

	server := &http.Server{
		Handler:      router,
		Addr:         *addr,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	// start HTTP server
	if *key != "" && *cert != "" {
		log.Println("[main] listening on https://", *addr)
		log.Fatal(server.ListenAndServeTLS(*cert, *key)) // blocking
	} else {
		log.Println("[main] listening on http://", *addr)
		log.Fatal(server.ListenAndServe()) // blocking
	}
}
