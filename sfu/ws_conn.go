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

type messageOut struct {
	Kind    string      `json:"kind"`
	Payload interface{} `json:"payload"`
}

type messageIn struct {
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
}

type joinPayload struct {
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

type controlPayload struct {
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Property string  `json:"property"`
	Value    float32 `json:"value"`
	Duration int     `json:"duration"`
}

// API

func newWsConn(unsafeConn *websocket.Conn) *wsConn {
	return &wsConn{sync.Mutex{}, unsafeConn, ""}
}

func (ws *wsConn) read() (m messageIn, err error) {
	err = ws.ReadJSON(&m)

	if err != nil && websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		log.Printf("[error] [ws user#%s] can't read: %v\n", ws.userId, err)
	}
	return
}

func (ws *wsConn) readJoin(origin string) (join joinPayload, err error) {
	var m messageIn

	// First message must be a join
	err = ws.ReadJSON(&m)
	if err != nil || m.Kind != "join" {
		return
	}

	err = json.Unmarshal([]byte(m.Payload), &join)
	join.origin = origin
	return
}

func (ws *wsConn) setUserId(userId string) {
	ws.Lock()
	defer ws.Unlock()

	ws.userId = userId
}

func (ws *wsConn) send(text string) (err error) {
	ws.Lock()
	defer ws.Unlock()

	m := &messageOut{Kind: text}
	if err := ws.Conn.WriteJSON(m); err != nil {
		log.Printf("[error] [ws user#%s] can't send: %v\n", ws.userId, err)
	}
	return
}

func (ws *wsConn) sendWithPayload(kind string, payload interface{}) (err error) {
	ws.Lock()
	defer ws.Unlock()

	m := &messageOut{
		Kind:    kind,
		Payload: payload,
	}
	if err := ws.Conn.WriteJSON(m); err != nil {
		log.Printf("[error] [ws user#%s] can't send with payload: %v\n", ws.userId, err)
	}
	return
}
