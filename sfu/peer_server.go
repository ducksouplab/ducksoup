package sfu

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

// Helper to make Gorilla Websockets threadsafe
type Conn struct {
	*websocket.Conn
	sync.Mutex
}

func (c *Conn) WriteJSON(v interface{}) error {
	c.Lock()
	defer c.Unlock()

	return c.Conn.WriteJSON(v)
}

// Handle incoming websockets
func NewPeerServer(unsafeConn *websocket.Conn) {
	conn := &Conn{unsafeConn, sync.Mutex{}}
	defer conn.Close()

	peerConnection := NewPeerConnection(conn)
	defer peerConnection.Close()

	// Add our new PeerConnection to global list
	tracksLock.Lock()
	peerConnections = append(peerConnections, peerConnectionState{peerConnection, conn})
	tracksLock.Unlock()

	signalingUpdate()

	message := &websocketMessage{}
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			return
		} else if err := json.Unmarshal(raw, &message); err != nil {
			log.Println(err)
			return
		}

		switch message.Event {
		case "candidate":
			candidate := webrtc.ICECandidateInit{}
			if err := json.Unmarshal([]byte(message.Data), &candidate); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				log.Println(err)
				return
			}

			if err := peerConnection.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}
		}
	}
}
