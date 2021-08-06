package sfu

import (
	"testing"
)

func newJoinPayload(origin, roomId, userId, namespace string, size int) joinPayload {
	return joinPayload{
		origin:    origin,
		RoomId:    roomId,
		UserId:    userId,
		Namespace: namespace,
		Size:      size,
	}
}

// Ticker could be stubbed to fasten test
func TestJoinRoom(t *testing.T) {

	t.Run("Ensure max capacity (2)", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "room-max-2", "user-1", "mirror", 2)
		joinPayload2 := newJoinPayload("https://origin", "room-max-2", "user-2", "mirror", 2)
		joinPayload3 := newJoinPayload("https://origin", "room-max-2", "user-3", "mirror", 2)

		_, err := JoinRoom(joinPayload1)

		if err != nil {
			t.Error("user #1 joined failed")
		}

		_, err = JoinRoom(joinPayload2)

		if err != nil {
			t.Error("user #2 joined failed")
		}

		_, err = JoinRoom(joinPayload3)
		if err.Error() != "full" {
			t.Error("room should be full for user #3")
		}
	})

	t.Run("Ban duplicates", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "room-dup", "user-1", "mirror", 2)
		joinPayloadDuplicate := newJoinPayload("https://origin", "room-dup", "user-1", "mirror", 2)

		_, err := JoinRoom(joinPayload1)

		if err != nil {
			t.Error("user #1 joined failed")
		}

		_, err = JoinRoom(joinPayloadDuplicate)

		if err.Error() != "duplicate" {
			t.Error("user #1 duplicate not banned")
		}
	})

	t.Run("Accept reconnections", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "room-re", "user-1", "mirror", 2)
		joinPayload2 := newJoinPayload("https://origin", "room-re", "user-2", "mirror", 2)
		joinPayload2bis := newJoinPayload("https://origin", "room-re", "user-2", "mirror", 2)

		room, _ := JoinRoom(joinPayload1)
		JoinRoom(joinPayload2)
		room.DisconnectUser("user-2")

		_, err := JoinRoom(joinPayload2bis)

		if err != nil {
			t.Error("room reconnection failed")
		}
	})

	t.Run("Reuse deleted room ids", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin", "room", "user-1", "mirror", 2)
		joinPayload2 := newJoinPayload("https://origin", "room", "user-2", "mirror", 2)
		joinPayloadReuse := newJoinPayload("https://origin", "room", "user-3", "mirror", 2)

		room, _ := JoinRoom(joinPayload1)
		JoinRoom(joinPayload2)
		room.delete()

		_, err := JoinRoom(joinPayloadReuse)
		if err != nil {
			t.Error("room id reuse failed")
		}
	})

	t.Run("Isolate origins", func(t *testing.T) {
		joinPayload1 := newJoinPayload("https://origin1", "room", "user-1", "mirror", 2)
		joinPayload2 := newJoinPayload("https://origin2", "room", "user-2", "mirror", 2)

		origin1Room, _ := JoinRoom(joinPayload1)
		origin2Room, _ := JoinRoom(joinPayload2)

		origin1RoomCount := origin1Room.userCount()
		origin2RoomCount := origin2Room.userCount()

		if origin1RoomCount != 1 || origin2RoomCount != 1 {
			t.Error("room origin clash")
		}
	})

}
