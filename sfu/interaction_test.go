package sfu

import (
	"testing"
)

// Ticker could be stubbed to fasten test
func TestJoinInteraction(t *testing.T) {

	t.Run("Ensure max capacity (2)", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "interaction-max-2", "user-1", "interaction", 2)
		joinPayload2 := newJoinPayload("https://origin", "interaction-max-2", "user-2", "interaction", 2)
		joinPayload3 := newJoinPayload("https://origin", "interaction-max-2", "user-3", "interaction", 2)

		_, _, err := interactionStoreSingleton.join(joinPayload1)

		if err != nil {
			t.Error("user #1 joined failed")
		}

		_, _, err = interactionStoreSingleton.join(joinPayload2)

		if err != nil {
			t.Error("user #2 joined failed")
		}

		_, _, err = interactionStoreSingleton.join(joinPayload3)
		if err.Error() != "full" {
			t.Error("interaction should be full for user #3")
		}
	})

	t.Run("Ban duplicates", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "interaction-dup", "user-1", "interaction", 2)
		joinPayloadDuplicate := newJoinPayload("https://origin", "interaction-dup", "user-1", "interaction", 2)

		_, _, err := interactionStoreSingleton.join(joinPayload1)

		if err != nil {
			t.Error("user #1 joined failed")
		}

		_, _, err = interactionStoreSingleton.join(joinPayloadDuplicate)

		if err.Error() != "duplicate" {
			t.Error("user #1 duplicate not banned")
		}
	})

	t.Run("Accept reconnections", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "interaction-re", "user-1", "interaction", 2)
		joinPayload2 := newJoinPayload("https://origin", "interaction-re", "user-2", "interaction", 2)
		joinPayload2bis := newJoinPayload("https://origin", "interaction-re", "user-2", "interaction", 2)

		interactionStoreSingleton.join(joinPayload1)
		i, _, _ := interactionStoreSingleton.join(joinPayload2)
		i.disconnectUser(joinPayload2.UserId)

		_, msg, err := interactionStoreSingleton.join(joinPayload2bis)

		if err != nil {
			t.Error("interaction reconnection failed")
		}
		if msg != "reconnection" {
			t.Error("join does not provide reconnection context")
		}
	})

	t.Run("Reuse deleted interaction ids", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "interaction", "user-1", "interaction", 2)
		joinPayload2 := newJoinPayload("https://origin", "interaction", "user-2", "interaction", 2)
		joinPayloadReuse := newJoinPayload("https://origin", "interaction", "user-3", "interaction", 2)

		i, _, _ := interactionStoreSingleton.join(joinPayload1)
		interactionStoreSingleton.join(joinPayload2)
		interactionStoreSingleton.delete(i)

		_, _, err := interactionStoreSingleton.join(joinPayloadReuse)
		if err != nil {
			t.Error("interaction id reuse failed")
		}
	})

	t.Run("Isolate origins", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin1", "interaction", "user-1", "interaction", 2)
		joinPayload2 := newJoinPayload("https://origin2", "interaction", "user-2", "interaction", 2)

		iOrigin1, _, _ := interactionStoreSingleton.join(joinPayload1)
		iOrigin2, _, _ := interactionStoreSingleton.join(joinPayload2)

		origin1InteractionCount := len(iOrigin1.connectedIndex)
		origin2InteractionCount := len(iOrigin2.connectedIndex)

		if origin1InteractionCount != 1 || origin2InteractionCount != 1 {
			t.Error("interaction origin clash")
		}
	})

}
