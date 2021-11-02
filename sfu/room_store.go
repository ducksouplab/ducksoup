package sfu

import (
	"errors"
	"log"
	"sync"

	"github.com/creamlab/ducksoup/types"
)

var (
	// sfu package exposed singleton
	rooms *roomStore
)

type roomStore struct {
	sync.Mutex
	index map[string]*room
}

func init() {
	rooms = newRoomStore()
}

func newRoomStore() *roomStore {
	return &roomStore{sync.Mutex{}, make(map[string]*room)}
}

func (rs *roomStore) join(join types.JoinPayload) (*room, error) {
	rs.Lock()
	defer rs.Unlock()

	qualifiedId := qualifiedId(join)
	userId := join.UserId

	if r, ok := rooms.index[qualifiedId]; ok {
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
			log.Printf("[info] [room#%s] [user#%s] joined\n", join.RoomId, userId)
			return r, nil
		}
	} else {
		log.Printf("[info] [room#%s] [user#%s] created for origin: %s\n", join.RoomId, userId, join.Origin)
		newRoom := newRoom(qualifiedId, join)
		rooms.index[qualifiedId] = newRoom
		return newRoom, nil
	}
}

func (rs *roomStore) delete(r *room) {
	rs.Lock()
	defer rs.Unlock()

	delete(rs.index, r.qualifiedId)
	log.Printf("[info] [room#%s] deleted\n", r.id)
}
