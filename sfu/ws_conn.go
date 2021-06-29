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
	userId string
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
	Room     string `json:"room"`
	Name     string `json:"name"`
	Duration int    `json:"duration"`
	UserId   string `json:"uid"`
	// optional
	Namespace  string `json:"namespace"`
	VideoCodec string `json:"videoCodec"`
	Size       int    `json:"size"`
	AudioFx    string `json:"audioFx"`
	VideoFx    string `json:"videoFx"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FrameRate  int    `json:"frameRate"`
}

// API

func NewWsConn(unsafeConn *websocket.Conn) *WsConn {
	return &WsConn{sync.Mutex{}, unsafeConn, ""}
}

func (w *WsConn) SetUserId(userId string) {
	w.Lock()
	defer w.Unlock()

	w.userId = userId
}

func (w *WsConn) Send(text string) (err error) {
	w.Lock()
	defer w.Unlock()

	m := &WsMessageOut{Kind: text}
	if err := w.Conn.WriteJSON(m); err != nil {
		log.Printf("[user %s error] WriteJSON: %v\n", w.userId, err)
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
		log.Printf("[user %s error] WriteJSON with payload: %v\n", w.userId, err)
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
