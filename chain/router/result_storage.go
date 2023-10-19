package router

import (
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"time"
)

type result struct {
	*routerv1.SendMessageResponse
	timeEntered time.Time
}

func (r *result) expired(expiryRange time.Duration) bool {
	return r.timeEntered.Add(expiryRange).After(time.Now())
}

type resultStorage struct {
	keepAlive time.Duration
	results   map[string]result
}

func newResultsStorage(keepAlive time.Duration) *resultStorage {
	return &resultStorage{
		keepAlive: keepAlive,
		results:   make(map[string]result),
	}
}

func (r *resultStorage) GetResult(hash string) (result, bool) {
	res, ok := r.results[hash]
	r.ClearStaleEntries()
	return res, ok
}

func (r *resultStorage) SetResult(msg *routerv1.SendMessageResponse) {
	r.results[msg.EvmTxHash] = result{
		SendMessageResponse: msg,
		timeEntered:         time.Now(),
	}
}

func (r *resultStorage) ClearStaleEntries() {
	for key, res := range r.results {
		if res.expired(r.keepAlive) {
			delete(r.results, key)
		}
	}
}
