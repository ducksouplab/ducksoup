package sfu

import (
	"errors"
	"sync"

	"github.com/ducksouplab/ducksoup/types"
	"github.com/rs/zerolog/log"
)

var (
	// sfu package exposed singleton
	interactionStoreSingleton *interactionStore
)

type interactionStore struct {
	sync.Mutex
	index map[string]*interaction
}

func init() {
	interactionStoreSingleton = newInteractionStore()
}

func newInteractionStore() *interactionStore {
	return &interactionStore{sync.Mutex{}, make(map[string]*interaction)}
}

func (is *interactionStore) join(join types.JoinPayload) (*interaction, error) {
	is.Lock()
	defer is.Unlock()

	id := generateId(join)
	userId := join.UserId

	if r, ok := interactionStoreSingleton.index[id]; ok {
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
			// interaction limit reached
			return nil, errors.New("full")
		} else {
			// new user joined existing interaction
			r.connectedIndex[userId] = true
			r.joinedCountIndex[userId] = 1
			log.Info().Str("context", "interaction").Str("namespace", join.Namespace).Str("interaction", join.InteractionName).Str("user", userId).Interface("payload", join).Msg("peer_joined")
			return r, nil
		}
	} else {
		newInteraction := newInteraction(id, join)
		log.Info().Str("context", "interaction").Str("namespace", join.Namespace).Str("interaction", join.InteractionName).Str("user", userId).Str("id", id).Str("origin", join.Origin).Msg("interaction_created")
		log.Info().Str("context", "interaction").Str("namespace", join.Namespace).Str("interaction", join.InteractionName).Str("user", userId).Interface("payload", join).Msg("peer_joined")
		interactionStoreSingleton.index[id] = newInteraction
		return newInteraction, nil
	}
}

func (is *interactionStore) delete(i *interaction) {
	is.Lock()
	defer is.Unlock()

	delete(is.index, i.id)
}
