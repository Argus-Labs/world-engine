package snapshot

import (
	"context"
	"sync"
	"time"

	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// Writer sends completed snapshots to storage. Writers report storage errors when Drain runs.
type Writer interface {
	// Write accepts an immutable snapshot. Do not call Write after Stop.
	// The caller must not change the snapshot after this call.
	Write(snap *cardinalv1.Snapshot)

	// Drain waits for submitted snapshots. Do not call Drain after Stop.
	// A newer stored snapshot can replace an older snapshot.
	Drain(ctx context.Context) error

	// Stop stops the writer. Call it one time. Call Drain first if the last snapshot must reach storage.
	Stop(ctx context.Context)
}

// -------------------------------------------------------------------------------------------------
// Synchronous writer
// -------------------------------------------------------------------------------------------------

// SyncWriter stores each snapshot synchronously before write returns.
type SyncWriter struct {
	storage Storage
	logger  zerolog.Logger
	lastErr error // Result of the newest storage attempt
}

var _ Writer = (*SyncWriter)(nil)

func NewSyncWriter(storage Storage, logger zerolog.Logger) *SyncWriter {
	return &SyncWriter{storage: storage, logger: logger}
}

// snapshotWriteTimeout limits each storage operation. This limit prevents one blocked operation
// from blocking all later snapshots.
const snapshotWriteTimeout = 2 * time.Second

// Write uses a background context so shutdown does not cancel the final snapshot immediately.
func (w *SyncWriter) Write(snap *cardinalv1.Snapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), snapshotWriteTimeout)
	defer cancel()

	err := w.storage.Store(ctx, snap)
	if err != nil {
		w.logger.Warn().Err(err).Uint64("tick_height", snap.GetTickHeight()).Msg("failed to store snapshot")
	} else {
		w.logger.Debug().Uint64("tick_height", snap.GetTickHeight()).Msg("published snapshot")
	}

	w.lastErr = err
}

// Drain returns the result of the newest completed storage attempt.
func (w *SyncWriter) Drain(_ context.Context) error {
	if w.lastErr == nil {
		return nil
	}
	return eris.Wrap(w.lastErr, "the newest snapshot did not reach storage")
}

// Stop does nothing because this writer has no goroutine.
func (w *SyncWriter) Stop(_ context.Context) {}

// -------------------------------------------------------------------------------------------------
// Asynchronous writer
// -------------------------------------------------------------------------------------------------

// AsyncWriter stores snapshots on a separate goroutine from the caller. One snapshot can
// wait during a Store call. A new snapshot replaces the waiting snapshot. The single goroutine also
// prevents writes from completing out of order. A process failure can lose a waiting snapshot or an
// incomplete Store call.
type AsyncWriter struct {
	storage Storage
	tel     *telemetry.Telemetry
	logger  zerolog.Logger

	cancel  context.CancelFunc // Stops the worker and the active storage operation
	done    chan struct{}      // Closes when the worker stops
	stopped bool               // Records that Stop ran. Mainly used for assertions
	wake    chan struct{}      // Tells the worker to read pending
	drain   chan chan error    // Sends a response channel to the worker

	mu      sync.Mutex           // Guards pending and dropped
	pending *cardinalv1.Snapshot // Newest snapshot that storage has not received
	dropped uint64               // Number of pending snapshots that a newer snapshot replaced
}

var _ Writer = (*AsyncWriter)(nil)

func NewAsyncWriter(storage Storage, tel *telemetry.Telemetry) *AsyncWriter {
	ctx, cancel := context.WithCancel(context.Background())
	w := &AsyncWriter{
		storage: storage,
		tel:     tel,
		logger:  tel.GetLogger("snapshot"),
		cancel:  cancel,
		wake:    make(chan struct{}, 1),
		drain:   make(chan chan error),
		done:    make(chan struct{}),
	}
	go w.loop(ctx)
	return w
}

func (w *AsyncWriter) loop(ctx context.Context) {
	defer close(w.done)

	var lastErr error
	for {
		var drainResult chan error
		select {
		case <-w.wake:
		case drainResult = <-w.drain:
		case <-ctx.Done():
			return
		}

		for {
			w.mu.Lock()
			snap := w.pending
			w.pending = nil
			w.mu.Unlock()

			if snap == nil {
				break
			}
			lastErr = w.store(ctx, snap)
		}

		if drainResult != nil {
			drainResult <- lastErr
		}
	}
}

// store makes one storage attempt. It converts a storage panic to an error.
// The conversion lets the writer continue.
func (w *AsyncWriter) store(ctx context.Context, snap *cardinalv1.Snapshot) (err error) {
	writeCtx, cancel := context.WithTimeout(ctx, snapshotWriteTimeout)
	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			err = eris.Errorf("snapshot storage panicked: %v", r)
			w.logger.Error().Err(err).Uint64("tick_height", snap.GetTickHeight()).
				Msg("recovered a panic from snapshot storage; the snapshot was not written")
			if w.tel != nil {
				w.tel.CaptureException(ctx, err)
			}
		}
	}()

	if err = w.storage.Store(writeCtx, snap); err != nil {
		w.logger.Warn().Err(err).Uint64("tick_height", snap.GetTickHeight()).Msg("failed to store snapshot")
	} else {
		w.logger.Debug().Uint64("tick_height", snap.GetTickHeight()).Msg("published snapshot")
	}
	return err
}

// Write sends a snapshot to the writer goroutine. It replaces a snapshot that is still waiting.
func (w *AsyncWriter) Write(snap *cardinalv1.Snapshot) {
	assert.That(!w.stopped, "cannot write after the snapshot writer stopped")

	w.mu.Lock()
	superseded := w.pending
	if superseded != nil {
		w.dropped++
	}

	w.pending = snap
	dropped := w.dropped
	w.mu.Unlock()

	select {
	case w.wake <- struct{}{}:
	default:
	}

	if superseded != nil {
		const snapshotDropLogEvery = 100
		if dropped == 1 || dropped%snapshotDropLogEvery == 0 {
			w.logger.Warn().
				Uint64("superseded_tick_height", superseded.GetTickHeight()).
				Uint64("tick_height", snap.GetTickHeight()).
				Uint64("dropped_total", dropped).
				Uint64("log_every", snapshotDropLogEvery).
				Msg("superseded a pending snapshot: storage is slower than the snapshot rate")
		}
	}
}

// Drain waits until storage accepts the newest submitted snapshot or an attempt fails.
// A context error also causes an error.
func (w *AsyncWriter) Drain(ctx context.Context) error {
	assert.That(!w.stopped, "cannot drain after the snapshot writer stopped")

	result := make(chan error, 1)
	select {
	case w.drain <- result:
	case <-ctx.Done():
		return eris.Wrap(ctx.Err(), "timed out waiting to drain snapshot writes")
	}

	select {
	case err := <-result:
		if err != nil {
			return eris.Wrap(err, "the newest snapshot did not reach storage")
		}
		return nil
	case <-ctx.Done():
		return eris.Wrap(ctx.Err(), "timed out waiting for pending snapshot writes to reach storage")
	}
}

// Stop prevents new writes and stops the writer goroutine.
// The context limits the wait for the goroutine.
func (w *AsyncWriter) Stop(ctx context.Context) {
	assert.That(!w.stopped, "snapshot writer stopped more than once")
	w.stopped = true

	w.mu.Lock()
	dropped := w.dropped
	w.mu.Unlock()

	if dropped > 0 {
		w.logger.Warn().Uint64("dropped_total", dropped).
			Msg("snapshots were superseded before they could be written during this run")
	}

	w.cancel()

	select {
	case <-w.done:
	case <-ctx.Done():
		w.logger.Warn().Msg("snapshot writer did not stop within the shutdown budget; abandoning it")
	}
}
