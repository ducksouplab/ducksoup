package sfu

import (
	"encoding/json"
	"log"
	"regexp"
	"sync"

	"github.com/creamlab/ducksoup/types"
	"github.com/gorilla/websocket"
)

const (
	MaxParsedLength = 50
)

// Helper to make Gorilla Websockets threadsafe
type wsConn struct {
	sync.Mutex
	*websocket.Conn
	roomId string
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

type controlPayload struct {
	Kind     string  `json:"kind"`
	Name     string  `json:"name"`
	Property string  `json:"property"`
	Value    float32 `json:"value"`
	Duration int     `json:"duration"`
}

// remove special characters like / . *
func parseString(str string) string {
	reg, _ := regexp.Compile("[^a-zA-Z0-9-]+")
	clean := reg.ReplaceAllString(str, "")
	if len(clean) == 0 {
		return "default"
	}
	if len(clean) > MaxParsedLength {
		return clean[0 : MaxParsedLength-1]
	}
	return clean
}

func parseWidth(join types.JoinPayload) (width int) {
	width = join.Width
	if width == 0 {
		width = defaultWidth
	}
	return
}

func parseHeight(join types.JoinPayload) (height int) {
	height = join.Height
	if height == 0 {
		height = defaultHeight
	}
	return
}

func parseFrameRate(join types.JoinPayload) (frameRate int) {
	frameRate = join.FrameRate
	if frameRate == 0 {
		frameRate = defaultFrameRate
	}
	return
}

// API

func newWsConn(unsafeConn *websocket.Conn) *wsConn {
	return &wsConn{sync.Mutex{}, unsafeConn, "", ""}
}

func (ws *wsConn) read() (m messageIn, err error) {
	err = ws.ReadJSON(&m)

	if err != nil && websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		log.Printf("[error] [room#%s] [user#%s] [ws] can't read: %v\n", ws.roomId, ws.userId, err)
	}
	return
}

func (ws *wsConn) readJoin(origin string) (join types.JoinPayload, err error) {
	var m messageIn

	// First message must be a join
	err = ws.ReadJSON(&m)
	if err != nil || m.Kind != "join" {
		return
	}

	err = json.Unmarshal([]byte(m.Payload), &join)
	// restrict to authorized values
	join.RoomId = parseString(join.RoomId)
	join.UserId = parseString(join.UserId)
	join.Namespace = parseString(join.Namespace)
	join.Width = parseWidth(join)
	join.Height = parseHeight(join)
	join.FrameRate = parseFrameRate(join)
	// add property
	join.Origin = origin

	return
}

func (ws *wsConn) setIds(roomId string, userId string) {
	ws.Lock()
	defer ws.Unlock()

	ws.roomId = roomId
	ws.userId = userId
}

func (ws *wsConn) send(text string) (err error) {
	ws.Lock()
	defer ws.Unlock()

	m := &messageOut{Kind: text}
	if err := ws.Conn.WriteJSON(m); err != nil {
		log.Printf("[error] [room#%s] [user#%s] [ws] can't send: %v\n", ws.roomId, ws.userId, err)
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
		log.Printf("[error] [room#%s] [user#%s] [ws] can't send with payload: %v\n", ws.roomId, ws.userId, err)
	}
	return
}
