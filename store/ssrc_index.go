package store

import (
	"sync"

	"github.com/rs/zerolog"
)

var (
	ssrcIndexSingleton *ssrcIndex
)

type ssrcLog struct {
	Kind        string
	Namespace   string
	Interaction string
	User        string
	logger      zerolog.Logger
}

type ssrcIndex struct {
	sync.Mutex
	index map[uint32]*ssrcLog
}

func init() {
	ssrcIndexSingleton = newIdsIndex()
}

func newIdsIndex() *ssrcIndex {
	return &ssrcIndex{sync.Mutex{}, make(map[uint32]*ssrcLog)}
}

func AddToSSRCIndex(ssrc uint32, kind, namespace, interaction, user string, logger zerolog.Logger) {
	ssrcIndexSingleton.Lock()
	defer ssrcIndexSingleton.Unlock()

	if _, ok := ssrcIndexSingleton.index[ssrc]; ok {
		logger.Error().
			Str("context", "interaction").
			Str("namespace", namespace).
			Str("interaction", interaction).
			Str("user", user).
			Msg("ssrc_index_failed")
	} else {
		newIds := &ssrcLog{
			Namespace:   namespace,
			Interaction: interaction,
			User:        user,
			Kind:        kind,
		}
		ssrcIndexSingleton.index[ssrc] = newIds
	}
}

func GetFromSSRCIndex(ssrc uint32) *ssrcLog {
	ssrcIndexSingleton.Lock()
	defer ssrcIndexSingleton.Unlock()

	if entry, ok := ssrcIndexSingleton.index[ssrc]; ok {
		return entry
	}
	return nil
}

func RemoveFromSSRCIndex(ssrc uint32) {
	ssrcIndexSingleton.Lock()
	defer ssrcIndexSingleton.Unlock()

	delete(ssrcIndexSingleton.index, ssrc)
}
