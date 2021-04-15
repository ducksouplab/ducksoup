package main

import (
	"flag"
	"log"
	"net/http"

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

	// Init other state
	log.SetFlags(0)

	// Server static files
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// websocket handler
	http.HandleFunc("/signaling", websocketHandler)

	// start HTTP server
	if *key != "" && *cert != "" {
		log.Println("Listening on https://", *addr)
		log.Fatal(http.ListenAndServeTLS(*addr, *cert, *key, nil))
	} else {
		log.Println("Listening on http://", *addr)
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

	sfu.NewPeer(unsafeConn)
}

func main() {
	go app()
	gst.StartMainLoop()
}
