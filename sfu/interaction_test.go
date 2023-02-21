package sfu

import (
	"testing"

	"github.com/ducksouplab/ducksoup/types"
)

func newJoinPayload(origin, interactionName, userId, namespace string, size int) types.JoinPayload {
	return types.JoinPayload{
		Origin:          origin,
		InteractionName: interactionName,
		UserId:          userId,
		Namespace:       namespace,
		Size:            size,
	}
}

// Ticker could be stubbed to fasten test
func TestJoinInteraction(t *testing.T) {

	t.Run("Ensure max capacity (2)", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "interaction-max-2", "user-1", "mirror", 2)
		joinPayload2 := newJoinPayload("https://origin", "interaction-max-2", "user-2", "mirror", 2)
		joinPayload3 := newJoinPayload("https://origin", "interaction-max-2", "user-3", "mirror", 2)

		_, err := interactionStoreSingleton.join(joinPayload1)

		if err != nil {
			t.Error("user #1 joined failed")
		}

		_, err = interactionStoreSingleton.join(joinPayload2)

		if err != nil {
			t.Error("user #2 joined failed")
		}

		_, err = interactionStoreSingleton.join(joinPayload3)
		if err.Error() != "full" {
			t.Error("interaction should be full for user #3")
		}
	})

	t.Run("Ban duplicates", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "interaction-dup", "user-1", "mirror", 2)
		joinPayloadDuplicate := newJoinPayload("https://origin", "interaction-dup", "user-1", "mirror", 2)

		_, err := interactionStoreSingleton.join(joinPayload1)

		if err != nil {
			t.Error("user #1 joined failed")
		}

		_, err = interactionStoreSingleton.join(joinPayloadDuplicate)

		if err.Error() != "duplicate" {
			t.Error("user #1 duplicate not banned")
		}
	})

	t.Run("Accept reconnections", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "interaction-re", "user-1", "mirror", 2)
		joinPayload2 := newJoinPayload("https://origin", "interaction-re", "user-2", "mirror", 2)
		joinPayload2bis := newJoinPayload("https://origin", "interaction-re", "user-2", "mirror", 2)

		i, _ := interactionStoreSingleton.join(joinPayload1)
		interactionStoreSingleton.join(joinPayload2)
		i.disconnectUser(joinPayload2.UserId)

		_, err := interactionStoreSingleton.join(joinPayload2bis)

		if err != nil {
			t.Error("interaction reconnection failed")
		}
	})

	t.Run("Reuse deleted interaction ids", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "interaction", "user-1", "mirror", 2)
		joinPayload2 := newJoinPayload("https://origin", "interaction", "user-2", "mirror", 2)
		joinPayloadReuse := newJoinPayload("https://origin", "interaction", "user-3", "mirror", 2)

		i, _ := interactionStoreSingleton.join(joinPayload1)
		interactionStoreSingleton.join(joinPayload2)
		interactionStoreSingleton.delete(i)

		_, err := interactionStoreSingleton.join(joinPayloadReuse)
		if err != nil {
			t.Error("interaction id reuse failed")
		}
	})

	t.Run("Isolate origins", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin1", "interaction", "user-1", "mirror", 2)
		joinPayload2 := newJoinPayload("https://origin2", "interaction", "user-2", "mirror", 2)

		iOrigin1, _ := interactionStoreSingleton.join(joinPayload1)
		iOrigin2, _ := interactionStoreSingleton.join(joinPayload2)

		origin1InteractionCount := iOrigin1.userCount()
		origin2InteractionCount := iOrigin2.userCount()

		if origin1InteractionCount != 1 || origin2InteractionCount != 1 {
			t.Error("interaction origin clash")
		}
	})

}
