package sfu

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Helper to make Gorilla Websockets threadsafe
type WsConn struct {
	sync.Mutex
	*websocket.Conn
}

type Message struct {
	Type    string `json:"type"`
	Payload string `json:"payload"`
}

type JoinPayload struct {
	Room string `json:"room"`
	User string `json:"user"`
}

func (w *WsConn) WriteJSON(v interface{}) (err error) {
	w.Lock()
	defer w.Unlock()

	if err := w.Conn.WriteJSON(v); err != nil {
		log.Println(err)
	}
	return
}

func (w *WsConn) ReadJoin() (joinPayload JoinPayload, err error) {
	var message Message

	// First message must be a join
	err = w.ReadJSON(&message)
	if err != nil || message.Type != "join" {
		return
	}

	err = json.Unmarshal([]byte(message.Payload), &joinPayload)
	return
}
