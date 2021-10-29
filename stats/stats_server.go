package stats

import (
	"time"

	"github.com/creamlab/ducksoup/sfu"
	"github.com/gorilla/websocket"
)

const (
	period = 900
)

type messageOut struct {
	Kind    string      `json:"kind"`
	Payload interface{} `json:"payload"`
}

// API

// handle incoming websockets
func RunStatsServer(ws *websocket.Conn) {
	defer ws.Close()

	ticker := time.NewTicker(period * time.Millisecond)

	for range ticker.C {
		payload := sfu.Inspect()
		if payload != nil {
			m := &messageOut{Kind: "update", Payload: payload}
			if err := ws.WriteJSON(m); err != nil {
				break
			}
		}
	}

}
