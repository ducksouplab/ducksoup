package sfu

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ducksouplab/ducksoup/types"
	"github.com/silently/wsmock"
)

func messageInWithPayload(kind string, payload any) messageIn {
	m, _ := json.Marshal(payload)
	return messageIn{kind, string(m)}
}

func TestRunPeerServer_Join(t *testing.T) {
	t.Run("fails when first message isn't of kind 'join'", func(t *testing.T) {
		conn, rec := wsmock.NewGorillaMockAndRecorder(t)
		go RunPeerServer("http://origin.test", conn)
		conn.Send(messageIn{"unknown", ""})
		rec.AssertFirstReceived(messageOut{Kind: "error-join"})
		rec.Run(200 * time.Millisecond)
	})

	t.Run("fails when join message does not contain an interactionName", func(t *testing.T) {
		conn, rec := wsmock.NewGorillaMockAndRecorder(t)
		go RunPeerServer("http://origin.test", conn)
		conn.Send(messageInWithPayload("join", types.JoinPayload{UserId: "user1"}))
		rec.AssertFirstReceived(messageOut{Kind: "error-join"})
		rec.Run(200 * time.Millisecond)
	})

	t.Run("fails when join message does not contain a userId", func(t *testing.T) {
		conn, rec := wsmock.NewGorillaMockAndRecorder(t)
		go RunPeerServer("http://origin.test", conn)
		conn.Send(messageInWithPayload("join", types.JoinPayload{InteractionName: "interaction1"}))
		rec.AssertFirstReceived(messageOut{Kind: "error-join"})
		rec.Run(200 * time.Millisecond)
	})

	t.Run("succeeds when join message is complete", func(t *testing.T) {
		conn, rec := wsmock.NewGorillaMockAndRecorder(t)
		go RunPeerServer("http://origin.test", conn)
		conn.Send(messageInWithPayload("join", types.JoinPayload{
			InteractionName: "interaction1",
			UserId:          "user1",
		}))
		rec.AssertReceivedContains("joined")
		rec.Run(200 * time.Millisecond)
	})
}

func TestRunPeerServer_Offer(t *testing.T) {
	t.Run("receives offer after join", func(t *testing.T) {
		conn, rec := wsmock.NewGorillaMockAndRecorder(t)
		go RunPeerServer("http://origin.test", conn)
		conn.Send(messageInWithPayload("join", types.JoinPayload{
			InteractionName: "interaction2",
			UserId:          "user2",
		}))
		rec.AssertReceivedContains("offer")
		rec.Run(1000 * time.Millisecond)
	})
}
