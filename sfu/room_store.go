package sfu

import (
	"errors"
	"sync"

	"github.com/ducksouplab/ducksoup/types"
	"github.com/rs/zerolog/log"
)

var (
	// sfu package exposed singleton
	roomStoreSingleton *roomStore
)

type roomStore struct {
	sync.Mutex
	index map[string]*room
}

func init() {
	roomStoreSingleton = newRoomStore()
}

func newRoomStore() *roomStore {
	return &roomStore{sync.Mutex{}, make(map[string]*room)}
}

func (rs *roomStore) join(join types.JoinPayload) (*room, error) {
	rs.Lock()
	defer rs.Unlock()

	qualifiedId := qualifiedId(join)
	userId := join.UserId

	if r, ok := roomStoreSingleton.index[qualifiedId]; ok {
		r.Lock()
		defer r.Unlock()
		connected, ok := r.connectedIndex[userId]
		if ok {
			// ok -> same user has previously connected
			if connected {
				// user is currently connected (second browser tab or device) -> forbidden
				return nil, errors.New("duplicate")
			} else {
				// reconnects (for instance: page reload)
				r.connectedIndex[userId] = true
				r.joinedCountIndex[userId]++
				return r, nil
			}
		} else if r.userCount() == r.size {
			// room limit reached
			return nil, errors.New("full")
		} else {
			// new user joined existing room
			r.connectedIndex[userId] = true
			r.joinedCountIndex[userId] = 1
			log.Info().Str("context", "room").Str("namespace", join.Namespace).Str("room", join.RoomId).Str("user", userId).Interface("payload", join).Msg("peer_joined")
			return r, nil
		}
	} else {
		newRoom := newRoom(qualifiedId, join)
		log.Info().Str("context", "room").Str("namespace", join.Namespace).Str("room", join.RoomId).Str("user", userId).Str("qualifiedId", qualifiedId).Str("origin", join.Origin).Msg("room_created")
		log.Info().Str("context", "room").Str("namespace", join.Namespace).Str("room", join.RoomId).Str("user", userId).Interface("payload", join).Msg("peer_joined")
		roomStoreSingleton.index[qualifiedId] = newRoom
		return newRoom, nil
	}
}

func (rs *roomStore) delete(r *room) {
	rs.Lock()
	defer rs.Unlock()

	delete(rs.index, r.qualifiedId)
}
