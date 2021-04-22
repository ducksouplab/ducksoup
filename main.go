package main

import (
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/creamlab/webrtc-transform/gst"
	"github.com/creamlab/webrtc-transform/sfu"
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

func app() {
	// Parse the flags passed to program
	flag.Parse()

	// Init logging
	log.SetFlags(log.Lmicroseconds)
	log.SetOutput(os.Stdout)

	// Server static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// websocket handler
	http.HandleFunc("/ws", websocketHandler)

	// start HTTP server
	if *key != "" && *cert != "" {
		log.Println("[main] listening on https://", *addr)
		log.Fatal(http.ListenAndServeTLS(*addr, *cert, *key, nil))
	} else {
		log.Println("[main] listening on http://", *addr)
		log.Fatal(http.ListenAndServe(*addr, nil))
	}
}

// Handle incoming websockets
func websocketHandler(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP request to Websocket
	unsafeConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	// When this frame returns close the Websocket
	defer unsafeConn.Close() //nolint

	sfu.RunPeerServer(unsafeConn)
}

func main() {
	go app()
	gst.StartMainLoop()
}
