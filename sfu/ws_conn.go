package sfu

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/types"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	MaxParsedLength = 50
)

// Helper to make Gorilla Websockets threadsafe
type wsConn struct {
	sync.Mutex
	*websocket.Conn
	createdAt time.Time
	userId    string
	roomId    string
	namespace string
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
	Name     string  `json:"name"`
	Property string  `json:"property"`
	Value    float32 `json:"value"`
	Duration int     `json:"duration"`
}

type polyControlPayload struct {
	Name     string `json:"name"`
	Property string `json:"property"`
	Kind     string `json:"kind"`
	Value    string `json:"value"`
	Duration int    `json:"duration"`
}

// remove special characters like / . *
func parseString(str string) string {
	reg, _ := regexp.Compile("[^a-zA-Z0-9-_]+")
	clean := reg.ReplaceAllString(str, "")
	if len(clean) == 0 {
		return "default"
	}
	if len(clean) > MaxParsedLength {
		return clean[0 : MaxParsedLength-1]
	}
	return clean
}

func parseVideoFormat(join types.JoinPayload) (videoFormat string) {
	videoFormat = join.VideoFormat
	if videoFormat != "VP8" && videoFormat != "H264" {
		videoFormat = defaultVideoFormat
	}
	return
}

func parseRecordingMode(join types.JoinPayload) (recordingMode string) {
	recordingMode = join.RecordingMode
	if recordingMode != "muxed" && recordingMode != "split" && recordingMode != "passthrough" && recordingMode != "none" {
		recordingMode = defaultRecordingMode
	}
	return
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
	return &wsConn{sync.Mutex{}, unsafeConn, time.Now(), "", "", ""}
}

func (ws *wsConn) logError() *zerolog.Event {
	return log.Error().Str("context", "peer").Str("namespace", ws.namespace).Str("room", ws.roomId).Str("user", ws.userId)
}

func (ws *wsConn) read() (m messageIn, err error) {
	err = ws.ReadJSON(&m)

	if err != nil && websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		ws.logError().Err(err).Msg("read_json_failed")
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
	join.VideoFormat = parseVideoFormat(join)
	join.RecordingMode = parseRecordingMode(join)
	join.Width = parseWidth(join)
	join.Height = parseHeight(join)
	join.FrameRate = parseFrameRate(join)
	// add property
	join.Origin = origin

	// bind fields
	ws.roomId = join.RoomId
	ws.userId = join.UserId
	ws.namespace = join.Namespace

	return
}

func (ws *wsConn) send(text string) (err error) {
	ws.Lock()
	defer ws.Unlock()

	m := &messageOut{Kind: text}
	if err := ws.Conn.WriteJSON(m); err != nil {
		ws.logError().Err(err).Str("out", fmt.Sprintf("%+v", m)).Msg("json_write_failed")
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
		ws.logError().Err(err).Str("out", fmt.Sprintf("%+v", m)).Msg("json_write_with_payload_failed")
	}
	return
}
