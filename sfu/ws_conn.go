package sfu

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/gorilla/websocket"
)

// Helper to make Gorilla Websockets threadsafe
type wsConn struct {
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
	RoomId   string `json:"roomId"`
	UserId   string `json:"userId"`
	Duration int    `json:"duration"`
	// optional
	Namespace  string `json:"namespace"`
	VideoCodec string `json:"videoCodec"`
	Size       int    `json:"size"`
	AudioFx    string `json:"audioFx"`
	VideoFx    string `json:"videoFx"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	FrameRate  int    `json:"frameRate"`
	// Not from JSON
	origin string
}

type ControlPayload struct {
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Property string  `json:"property"`
	Value    float32 `json:"value"`
	Duration int     `json:"duration"`
}

// API

func NewWsConn(unsafeConn *websocket.Conn) *wsConn {
	return &wsConn{sync.Mutex{}, unsafeConn, ""}
}

func (ws *wsConn) SetUserId(userId string) {
	ws.Lock()
	defer ws.Unlock()

	ws.userId = userId
}

func (ws *wsConn) Send(text string) (err error) {
	ws.Lock()
	defer ws.Unlock()

	m := &WsMessageOut{Kind: text}
	if err := ws.Conn.WriteJSON(m); err != nil {
		log.Printf("[user %s error] WriteJSON: %v\n", ws.userId, err)
	}
	return
}

func (ws *wsConn) SendWithPayload(kind string, payload interface{}) (err error) {
	ws.Lock()
	defer ws.Unlock()

	m := &WsMessageOut{
		Kind:    kind,
		Payload: payload,
	}
	if err := ws.Conn.WriteJSON(m); err != nil {
		log.Printf("[user %s error] WriteJSON with payload: %v\n", ws.userId, err)
	}
	return
}

func (ws *wsConn) ReadJoin(origin string) (joinPayload JoinPayload, err error) {
	var m WsMessageIn

	// First message must be a join
	err = ws.ReadJSON(&m)
	if err != nil || m.Kind != "join" {
		return
	}

	err = json.Unmarshal([]byte(m.Payload), &joinPayload)
	joinPayload.origin = origin
	return
}
