package sfu

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ducksouplab/ducksoup/types"
	"github.com/silently/wsmock"
)

func joinMessageIn() messageIn {
	payload, _ := json.Marshal(types.JoinPayload{})
	return messageIn{"join", string(payload)}
}

func TestRunPeerServer(t *testing.T) {

	t.Run("...", func(t *testing.T) {
		conn, rec := wsmock.NewGorillaMockAndRecorder(t)
		go RunPeerServer("origin", "href", conn)
		conn.Send(joinMessageIn())
		rec.AssertReceivedContains("joined")
		rec.AssertReceivedContains("offer")
		rec.Run(3000 * time.Millisecond)
	})
}
