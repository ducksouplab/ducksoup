package sfu

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v3"
)

// Helper to make Gorilla Websockets threadsafe
type WebsocketConn struct {
	*websocket.Conn
	sync.Mutex
}

type Message struct {
	Event string `json:"event"`
	Data  string `json:"data"`
}

func (w *WebsocketConn) WriteJSON(v interface{}) error {
	w.Lock()
	defer w.Unlock()

	return w.Conn.WriteJSON(v)
}

// Handle incoming websockets
func NewPeerServer(unsafeConn *websocket.Conn) {
	room := GetRoom("main")

	wsConn := &WebsocketConn{unsafeConn, sync.Mutex{}}
	defer wsConn.Close()

	rtcConn := NewPeerConnection(room, wsConn)
	defer rtcConn.Close()

	message := &Message{}
	for {
		_, raw, err := wsConn.ReadMessage()
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

			if err := rtcConn.AddICECandidate(candidate); err != nil {
				log.Println(err)
				return
			}
		case "answer":
			answer := webrtc.SessionDescription{}
			if err := json.Unmarshal([]byte(message.Data), &answer); err != nil {
				log.Println(err)
				return
			}

			if err := rtcConn.SetRemoteDescription(answer); err != nil {
				log.Println(err)
				return
			}
		}
	}
}
