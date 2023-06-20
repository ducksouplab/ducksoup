package store

import (
	"sync"

	"github.com/rs/zerolog"
)

var (
	ssrcIndexSingleton ssrcIndex
)

// Used to provide more info to RTCP packetdump
type ssrcLog struct {
	Kind        string
	Namespace   string
	Interaction string
	User        string
}

type ssrcIndex struct {
	sync.Mutex
	index map[uint32]ssrcLog
}

func init() {
	ssrcIndexSingleton = ssrcIndex{sync.Mutex{}, make(map[uint32]ssrcLog)}
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
			Msg("ssrc_index_duplicate")
	} else {
		newLog := ssrcLog{
			Namespace:   namespace,
			Interaction: interaction,
			User:        user,
			Kind:        kind,
		}
		ssrcIndexSingleton.index[ssrc] = newLog
	}
}

func GetFromSSRCIndex(ssrc uint32) (l ssrcLog, ok bool) {
	ssrcIndexSingleton.Lock()
	defer ssrcIndexSingleton.Unlock()

	l, ok = ssrcIndexSingleton.index[ssrc]
	return
}

func RemoveFromSSRCIndex(ssrc uint32) {
	ssrcIndexSingleton.Lock()
	defer ssrcIndexSingleton.Unlock()

	delete(ssrcIndexSingleton.index, ssrc)
}
