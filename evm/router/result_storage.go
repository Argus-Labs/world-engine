package router

import (
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	routerv1 "pkg.world.dev/world-engine/rift/router/v1"
)

/*
	Results storage is a rudimentary storage system. It uses a sync.Map to store things, and records the time the result
    was placed in storage. Each time a Result is fetched, we make a call to clear the stale entries.

*/

type Result struct {
	*routerv1.SendMessageResponse
	timeEntered time.Time
}

func (r Result) expired(expiryRange time.Duration) bool {
	return time.Now().After(r.timeEntered.Add(expiryRange))
}

type ResultStorage interface { //nolint:decorder
	Result(key string) (Result, bool)
	SetResult(msg *routerv1.SendMessageResponse)
}

type resultStorageMemory struct { //nolint:decorder
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
	defer r.clearStaleEntries()
	if !ok {
		return Result{}, ok
	}
	if res, ok := res.(Result); ok {
		return res, true
	}
	return Result{}, false
}

func (r *resultStorageMemory) SetResult(msg *routerv1.SendMessageResponse) {
	result := Result{msg, time.Now()}
	log.Debug().Msgf("storing result for tx %q: result: %s", msg.GetEvmTxHash(), result.String())
	r.results.Store(msg.GetEvmTxHash(), result)
}

func (r *resultStorageMemory) clearStaleEntries() {
	r.results.Range(func(key, value any) bool {
		res, _ := value.(Result)
		if res.expired(r.keepAlive) {
			log.Debug().Msgf("result expired: deleting result for %v", key)
			r.results.Delete(key)
		}
		return true
	})
}
