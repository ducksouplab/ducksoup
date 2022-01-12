package store

import (
	"sync"

	"github.com/rs/zerolog/log"
)

var (
	ssrcIndexSingleton *ssrcIndex
)

type ssrcLog struct {
	Kind      string
	Namespace string
	Room      string
	User      string
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

func AddToSSRCIndex(ssrc uint32, kind, namespace, room, user string) {
	ssrcIndexSingleton.Lock()
	defer ssrcIndexSingleton.Unlock()

	if _, ok := ssrcIndexSingleton.index[ssrc]; ok {
		log.Error().
			Str("context", "room").
			Str("namespace", namespace).
			Str("room", room).
			Str("user", user).
			Msg("ssrc_index_failed")
	} else {
		newIds := &ssrcLog{
			Namespace: namespace,
			Room:      room,
			User:      user,
			Kind:      kind,
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
