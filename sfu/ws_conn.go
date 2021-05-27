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
	Room     string `json:"room"`
	Name     string `json:"name"`
	Duration int    `json:"duration"`
	UserId   string `json:"uid"`
	Proc     bool   `json:"proc"`
	H264     bool   `json:"h264"`
}

// API

func (w *WsConn) Send(text string) (err error) {
	w.Lock()
	defer w.Unlock()

	message := &Message{
		Type: text,
	}
	if err := w.Conn.WriteJSON(message); err != nil {
		log.Println(err)
	}
	return
}

func (w *WsConn) SendJSON(v interface{}) (err error) {
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
