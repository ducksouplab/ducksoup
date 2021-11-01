package gst

import (
	"log"
	"sync"
)

var (
	// sfu package exposed singleton
	pipelines *pipelineStore
)

type pipelineStore struct {
	sync.Mutex
	index map[string]*Pipeline
}

func init() {
	pipelines = newPipelinetore()
}

func newPipelinetore() *pipelineStore {
	return &pipelineStore{sync.Mutex{}, make(map[string]*Pipeline)}
}

func (ps *pipelineStore) add(p *Pipeline) {
	ps.Lock()
	defer ps.Unlock()

	ps.index[p.id] = p
}

func (ps *pipelineStore) find(id string) (p *Pipeline, ok bool) {
	ps.Lock()
	defer ps.Unlock()

	p, ok = ps.index[id]
	return
}

func (ps *pipelineStore) delete(id string) {
	ps.Lock()
	defer ps.Unlock()

	pipeline, ok := ps.index[id]
	if ok {
		log.Printf("[info] [room#%s] [user#%s] [output_track#%s] [pipeline] stop done\n", pipeline.join.RoomId, pipeline.join.UserId, id)

	}

	delete(ps.index, id)
}
