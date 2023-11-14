package router

import (
	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
	"sync"
	"time"
)

type ResultStorage interface {
	Result(key string) (Result, bool)
	SetResult(msg *routerv1.SendMessageResponse)
}

type Result struct {
	*routerv1.SendMessageResponse
	timeEntered time.Time
}

func (r Result) expired(expiryRange time.Duration) bool {
	return time.Now().After(r.timeEntered.Add(expiryRange))
}

type resultStorageMemory struct {
	keepAlive time.Duration
	results   *sync.Map // map[string]Result
}

func NewMemoryResultStorage(keepAlive time.Duration) ResultStorage {
	return &resultStorageMemory{
		keepAlive: keepAlive,
		results:   new(sync.Map),
	}
}

func (r *resultStorageMemory) Result(hash string) (Result, bool) {
	res, ok := r.results.Load(hash)
	r.clearStaleEntries()
	if !ok {
		return Result{}, ok
	}
	return res.(Result), ok
}

func (r *resultStorageMemory) SetResult(msg *routerv1.SendMessageResponse) {
	r.results.Store(msg.EvmTxHash, Result{msg, time.Now()})
}

func (r *resultStorageMemory) clearStaleEntries() {
	r.results.Range(func(key, value any) bool {
		res, _ := value.(Result)
		if res.expired(r.keepAlive) {
			r.results.Delete(key)
		}
		return true
	})
}
