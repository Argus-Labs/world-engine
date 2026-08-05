package snapshot

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// storeTimeout bounds a single background write. It is deliberately generous: the write no longer
// blocks the tick loop, so the timeout only guards against a hung backend leaking goroutines.
const storeTimeout = 30 * time.Second

// AsyncStorage decorates a Storage so Store never blocks: writes happen on a background
// goroutine, one at a time (parallel writes to the same key could land older-over-newer).
// At most one snapshot waits its turn — a newer Store replaces it, so the freshest state
// is always the one persisted.
//
// Store must not be called concurrently. Flush must come after the last Store. Load delegates.
type AsyncStorage struct {
	inner  Storage
	logger zerolog.Logger

	mu      sync.Mutex
	pending *Snapshot     // newest unsent snapshot; replaced wholesale by a newer Store
	done    chan struct{} // non-nil while a drain goroutine is active; closed when it exits
}

var _ Storage = (*AsyncStorage)(nil)

// NewAsync wraps inner so Store returns immediately and writes happen in the background.
func NewAsync(inner Storage, logger zerolog.Logger) *AsyncStorage {
	return &AsyncStorage{inner: inner, logger: logger}
}

// Store hands the snapshot to the background writer and returns nil immediately. Write errors are
// logged, not returned (snapshots are best-effort; the next Store retries with fresher state).
func (a *AsyncStorage) Store(_ context.Context, snap *Snapshot) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.pending != nil {
		a.logger.Debug().
			Uint64("superseded_tick", a.pending.TickHeight).
			Uint64("tick", snap.TickHeight).
			Msg("snapshot superseded before it was written")
	}
	a.pending = snap

	if a.done == nil {
		a.done = make(chan struct{})
		go a.drain(a.done)
	}
	return nil
}

// drain writes pending snapshots until none remain, then exits. At most one drain runs at a time;
// done is closed on exit so Flush can wait for it.
func (a *AsyncStorage) drain(done chan struct{}) {
	defer close(done)
	for {
		a.mu.Lock()
		snap := a.pending
		a.pending = nil
		if snap == nil {
			a.done = nil
			a.mu.Unlock()
			return
		}
		a.mu.Unlock()

		ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
		if err := a.inner.Store(ctx, snap); err != nil {
			a.logger.Warn().Err(err).Uint64("tick", snap.TickHeight).Msg("failed to store snapshot")
		}
		cancel()
	}
}

// Load delegates to the wrapped storage. It is only called at boot, before any Store.
func (a *AsyncStorage) Load(ctx context.Context) (*Snapshot, error) {
	return a.inner.Load(ctx)
}

// Flush blocks until every submitted snapshot has been written (or failed), or ctx expires. Call it
// after the final Store so shutdown doesn't abandon an in-flight write.
func (a *AsyncStorage) Flush(ctx context.Context) error {
	a.mu.Lock()
	done := a.done
	a.mu.Unlock()
	if done == nil {
		return nil
	}

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
