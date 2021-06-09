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

type WsMessageOut struct {
	Kind    string      `json:"kind"`
	Payload interface{} `json:"payload"`
}

type WsMessageIn struct {
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
}

type JoinPayload struct {
	Room       string `json:"room"`
	Name       string `json:"name"`
	Duration   int    `json:"duration"`
	UserId     string `json:"uid"`
	Proc       bool   `json:"proc"`
	VideoCodec string `json:"videoCodec"`
	Size       int    `json:"size"`
}

// API

func (w *WsConn) Send(text string) (err error) {
	w.Lock()
	defer w.Unlock()

	m := &WsMessageOut{Kind: text}
	if err := w.Conn.WriteJSON(m); err != nil {
		log.Println(err)
	}
	return
}

func (w *WsConn) SendWithPayload(kind string, payload interface{}) (err error) {
	w.Lock()
	defer w.Unlock()

	m := &WsMessageOut{
		Kind:    kind,
		Payload: payload,
	}
	if err := w.Conn.WriteJSON(m); err != nil {
		log.Println(err)
	}
	return
}

func (w *WsConn) ReadJoin() (joinPayload JoinPayload, err error) {
	var m WsMessageIn

	// First message must be a join
	err = w.ReadJSON(&m)
	if err != nil || m.Kind != "join" {
		return
	}

	err = json.Unmarshal([]byte(m.Payload), &joinPayload)
	return
}
