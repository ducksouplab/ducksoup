package sfu

import (
	"encoding/json"
	"errors"
	"regexp"
	"sync"
	"time"

	"github.com/ducksouplab/ducksoup/types"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	ws "github.com/silently/wsmock"
)

const (
	MaxParsedLength = 50
)

// Helper to make Gorilla Websockets threadsafe
type wsConn struct {
	sync.Mutex
	ws.IGorilla
	createdAt       time.Time
	userId          string
	interactionName string
	namespace       string
	ps              *peerServer
	logger          zerolog.Logger
}

type messageOut struct {
	Kind    string `json:"kind"`
	Payload any    `json:"payload"`
}

type messageIn struct {
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
}

type controlPayload struct {
	UserId   string  `json:"userId"`
	Name     string  `json:"name"`
	Property string  `json:"property"`
	Value    float32 `json:"value"`
	Duration int     `json:"duration"`
	// not from unmarshalling
	fromUserId string
}

type polyControlPayload struct {
	Name     string `json:"name"`
	Property string `json:"property"`
	Kind     string `json:"kind"`
	Value    string `json:"value"`
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

func parseFramerate(join types.JoinPayload) (framerate int) {
	framerate = join.Framerate
	if framerate == 0 {
		framerate = defaultFramerate
	}
	return
}

// API

func newWsConn(unsafeConn ws.IGorilla) *wsConn {
	logger := log.With().Str("context", "peer").Logger() // default logger

	return &wsConn{sync.Mutex{}, unsafeConn, time.Now(), "", "", "", nil, logger}
}

func (ws *wsConn) setLogger(logger zerolog.Logger) {
	ws.logger = logger
}

func (ws *wsConn) logError() *zerolog.Event {
	return ws.logger.Error().Str("user", ws.userId)
}

// peer server has not been created yet
func (ws *wsConn) readJoin(origin string) (join types.JoinPayload, err error) {
	var m messageIn

	// First message must be a join
	err = ws.ReadJSON(&m)

	if err != nil {
		// no need to ws.send an error if we can't read
		return
	} else if m.Kind != "join" {
		err = errors.New("wrong_join_payload_kind")
		// we don't use send method since it may try to close not created peer server
		m := &messageOut{Kind: "error-join"}
		ws.WriteJSON(m)
		return
	}

	err = json.Unmarshal([]byte(m.Payload), &join)

	// restrict to authorized values
	join.Namespace = parseString(join.Namespace)
	join.InteractionName = parseString(join.InteractionName)
	join.UserId = parseString(join.UserId)
	join.VideoFormat = parseVideoFormat(join)
	join.RecordingMode = parseRecordingMode(join)
	join.Width = parseWidth(join)
	join.Height = parseHeight(join)
	join.Framerate = parseFramerate(join)
	// add property
	join.Origin = origin

	// bind fields
	ws.interactionName = join.InteractionName
	ws.userId = join.UserId
	ws.namespace = join.Namespace
	return
}

func (ws *wsConn) connectPeerServer(ps *peerServer) {
	ws.Lock()
	defer ws.Unlock()

	ws.ps = ps
}

func (ws *wsConn) receive() (m messageIn, err error) {
	err = ws.ReadJSON(&m)

	if err != nil && websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		ws.ps.close("ws_read_error")
		ws.logError().Err(err).Msg("read_json_failed")
	}
	return
}

func (ws *wsConn) send(text string) (err error) {
	ws.Lock()
	defer ws.Unlock()

	m := &messageOut{Kind: text}

	if err = ws.WriteJSON(m); err != nil {
		ws.ps.close("ws_write_error")
	}
	return
}

func (ws *wsConn) sendWithPayload(kind string, payload any) (err error) {
	ws.Lock()
	defer ws.Unlock()

	m := &messageOut{
		Kind:    kind,
		Payload: payload,
	}

	if err = ws.WriteJSON(m); err != nil {
		ws.ps.close("ws_write_error")
	}
	return
}
