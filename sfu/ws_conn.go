package sfu

import (
	"encoding/json"
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

func NewWsConn(unsafeConn *websocket.Conn) *WsConn {
	return &WsConn{sync.Mutex{}, unsafeConn}
}

func (w *WsConn) WriteJSON(v interface{}) error {
	w.Lock()
	defer w.Unlock()

	return w.Conn.WriteJSON(v)
}

func (w *WsConn) ReadJoin() (roomName string, userName string, err error) {
	var message Message
	var joinPayload JoinPayload

	// First message must be a join
	err = w.ReadJSON(&message)
	if err != nil || message.Type != "join" {
		return
	}
	if err = json.Unmarshal([]byte(message.Payload), &joinPayload); err == nil {
		roomName = joinPayload.Room
		userName = joinPayload.User
	}
	return
}
