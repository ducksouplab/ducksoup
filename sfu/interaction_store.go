package sfu

import (
	"sync"

	"github.com/ducksouplab/ducksoup/types"
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
func (is *interactionStore) join(jp types.JoinPayload) (*interaction, string, error) {
	is.Lock()
	defer is.Unlock()

	interactionId := generateId(jp)

	if i, ok := interactionStoreSingleton.index[interactionId]; ok {
		msg, err := i.join(jp)
		return i, msg, err
	} else {
		// new user creates interaction
		i := newInteraction(interactionId, jp)
		interactionStoreSingleton.index[interactionId] = i
		return i, "new_interaction", nil
	}
}

func (is *interactionStore) delete(i *interaction) {
	is.Lock()
	defer is.Unlock()

	delete(is.index, i.id)
}
