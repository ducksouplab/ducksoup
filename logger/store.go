package sfu

import (
	"sync"

	"github.com/rs/zerolog"
)

var (
	// sfu package exposed singleton
	singleton *loggerStore
)

type loggerStore struct {
	sync.Mutex
	index map[string]*zerolog.Logger // index by interaction random Id
}

func init() {
	singleton = newLoggerStore()
}

func newLoggerStore() *loggerStore {
	return &loggerStore{
		sync.Mutex{},
		make(map[string]*zerolog.Logger),
	}
}

func SetLogger(randomId string, l *zerolog.Logger) {
	singleton.Lock()
	defer singleton.Unlock()

	singleton.index[randomId] = l
}

func DeleteLogger(randomId string) {
	singleton.Lock()
	defer singleton.Unlock()

	delete(singleton.index, randomId)
}

func GetLogger(randomId string) (l *zerolog.Logger, ok bool) {
	singleton.Lock()
	defer singleton.Unlock()

	l, ok = singleton.index[randomId]
	return
}
