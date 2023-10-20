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
	return time.Now().After(r.timeEntered.Add(expiryRange))
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
	r.clearStaleEntries()
	return res, ok
}

func (r *resultStorage) SetResult(msg *routerv1.SendMessageResponse) {
	r.results[msg.EvmTxHash] = result{
		SendMessageResponse: msg,
		timeEntered:         time.Now(),
	}
}

func (r *resultStorage) clearStaleEntries() {
	for key, res := range r.results {
		if res.expired(r.keepAlive) {
			delete(r.results, key)
		}
	}
}
