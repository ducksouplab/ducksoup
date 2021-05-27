package server

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/creamlab/webrtc-transform/sfu"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var (
	addr     = flag.String("addr", ":8080", "http service address")
	cert     = flag.String("cert", "", "cert file")
	key      = flag.String("key", "", "key file")
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

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
	router.PathPrefix("/1on1/").Handler(http.StripPrefix("/1on1/", http.FileServer(http.Dir("./front/static/pages/1on1/"))))
	router.PathPrefix("/embed/").Handler(http.StripPrefix("/embed/", http.FileServer(http.Dir("./front/static/pages/embed/"))))
	router.PathPrefix("/test/").Handler(http.StripPrefix("/test/", http.FileServer(http.Dir("./front/static/pages/test/"))))

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
		log.Fatal(server.ListenAndServeTLS(*addr, *cert)) // blocking
	} else {
		log.Println("[main] listening on http://", *addr)
		log.Fatal(server.ListenAndServe()) // blocking
	}
}
