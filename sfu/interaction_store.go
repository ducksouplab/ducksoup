package sfu

import (
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

// last return value provides additional context
func (is *interactionStore) join(join types.JoinPayload) (*interaction, string, error) {
	is.Lock()
	defer is.Unlock()

	interactionId := generateId(join)
	userId := join.UserId

	if i, ok := interactionStoreSingleton.index[interactionId]; ok {
		msg, err := i.join(join)
		return i, msg, err
	} else {
		// new user creates interaction
		newInteraction := newInteraction(interactionId, join)
		log.Info().Str("context", "interaction").Str("namespace", join.Namespace).Str("interaction", join.InteractionName).Str("user", userId).Str("id", interactionId).Str("origin", join.Origin).Msg("interaction_created")
		log.Info().Str("context", "interaction").Str("namespace", join.Namespace).Str("interaction", join.InteractionName).Str("user", userId).Interface("payload", join).Msg("peer_joined")
		interactionStoreSingleton.index[interactionId] = newInteraction
		return newInteraction, "new-interaction", nil
	}
}

func (is *interactionStore) delete(i *interaction) {
	is.Lock()
	defer is.Unlock()

	delete(is.index, i.id)
}
