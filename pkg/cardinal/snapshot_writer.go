package cardinal

import (
	"context"
	"sync"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// snapshotWriteTimeout bounds a single Store call, so a backend that never answers cannot hold the
// writer — and with it every later snapshot — forever. It is the same 2s the inline write path used
// before the upload moved off the tick goroutine; what changed is where the clock starts, not how
// long a write may take.
const snapshotWriteTimeout = 2 * time.Second

// snapshotWriter takes a finished snapshot envelope from the tick goroutine and gets it to storage.
//
// The tick goroutine must never wait on a backend, so write() takes no context and returns no
// error: snapshot writes are best-effort by contract (see World.snapshot), and an error the caller
// cannot act on is a lie about who handles it. Failures are logged by the writer and nothing else
// happens — the world keeps ticking and the next snapshot retries.
type snapshotWriter interface {
	// write hands over a snapshot envelope. It never blocks on storage.
	//
	// Ownership: the writer may hold the message past the call, so the caller must treat it as
	// frozen from here on. Cardinal satisfies that by construction — every envelope wraps a world
	// state ToProto just built out of fresh slices, and nothing mutates it afterwards.
	write(snap *cardinalv1.Snapshot)

	// drain waits until every envelope handed to write before this call has either reached storage
	// or been superseded by a later one that has. It returns an error if ctx expires first, or if
	// the writer stopped with writes still outstanding — never blocking past ctx.
	drain(ctx context.Context) error

	// stop shuts the writer down. Snapshots handed to write afterwards are dropped, so callers that
	// care about the last one drain first. Bounded by ctx.
	stop(ctx context.Context)
}

// storeSnapshot performs one write attempt and reports it. Failures are logged, never returned:
// snapshot writes are best-effort, and the caller of a snapshot write has nothing to do with an
// error but log it.
func storeSnapshot(
	ctx context.Context,
	storage snapshot.Storage,
	snap *cardinalv1.Snapshot,
	logger *zerolog.Logger,
) {
	if err := storage.Store(ctx, snap); err != nil {
		logger.Warn().Err(err).Uint64("tick_height", snap.GetTickHeight()).Msg("failed to store snapshot")
		return
	}
	logger.Debug().Uint64("tick_height", snap.GetTickHeight()).Msg("published snapshot")
}

// -------------------------------------------------------------------------------------------------
// Inline writer
// -------------------------------------------------------------------------------------------------

// inlineSnapshotWriter writes on the caller's goroutine, which is what the snapshot path did before
// the upload moved off the tick loop. It exists for the callers that need a write to have happened
// by the time write() returns:
//
//   - DST (see World.useInlineSnapshotStorage). A background writer would make DST irreproducible in
//     two separate ways. Latest-wins means the set of snapshots that actually reach storage depends
//     on how the upload goroutine interleaves with the tick loop, so the same seed could store a
//     different snapshot from run to run — and DST's whole premise is that a seed reproduces a run.
//     On top of that the in-memory backend asserts on a *testing.T, which only the test goroutine
//     may do. Determinism is worth more to DST than the tick latency the async writer buys, and DST
//     ticks against an in-memory map where there is no latency to buy back.
//   - Unit tests that assert on what storage holds right after a snapshot.
type inlineSnapshotWriter struct {
	storage snapshot.Storage
	logger  zerolog.Logger
}

var _ snapshotWriter = (*inlineSnapshotWriter)(nil)

func newInlineSnapshotWriter(storage snapshot.Storage, logger zerolog.Logger) *inlineSnapshotWriter {
	return &inlineSnapshotWriter{storage: storage, logger: logger}
}

// write stores the snapshot before returning. The timeout hangs off context.Background() rather than
// a caller context so that a write started during shutdown, when the run context is already
// cancelled, still gets its full budget — the final snapshot is the one that matters most.
func (w *inlineSnapshotWriter) write(snap *cardinalv1.Snapshot) {
	ctx, cancel := context.WithTimeout(context.Background(), snapshotWriteTimeout)
	defer cancel()
	storeSnapshot(ctx, w.storage, snap, &w.logger)
}

// drain is a no-op: write() already stored everything it was handed.
func (w *inlineSnapshotWriter) drain(_ context.Context) error { return nil }

// stop is a no-op: there is nothing running to stop.
func (w *inlineSnapshotWriter) stop(_ context.Context) {}

// -------------------------------------------------------------------------------------------------
// Async writer
// -------------------------------------------------------------------------------------------------

// asyncSnapshotWriter uploads snapshots on a goroutine of its own, so a snapshot tick costs the
// shard loop a mutex instead of a marshal plus a network round trip. That is the entire point: the
// snapshot tick is a tail-latency spike, and the upload is the unbounded part of it.
//
// SINGLE-FLIGHT, LATEST-WINS. At most one Store runs at a time, and a snapshot arriving while one is
// in flight REPLACES whatever was waiting rather than queueing behind it. Two properties fall out:
//
//   - Storage never ends up holding an older snapshot than one it already accepted. One goroutine
//     issues every Store, in submission order, and never concurrently — so no two writes can land
//     out of order.
//   - A backend slower than the snapshot rate costs bounded memory: one pending world-state graph
//     (~1 MB serialized at 5000 entities), not a queue that grows until the process dies. Queueing
//     would also be pointless work — every queued snapshot but the last is already superseded by
//     the time it would be written.
//
// DURABILITY TRADE, ACCEPTED DELIBERATELY. A snapshot replaced while pending is never written, and a
// crash between a tick and the completion of its upload loses that snapshot. The next one still
// lands, and per-write atomicity is untouched (JetStream commits the new object then purges the old
// chunks; S3 replaces the object atomically), so storage always holds some complete, valid snapshot
// — just possibly an older one than the inline path would have left. Every drop is counted and
// logged so "my snapshots are stale" stays a falsifiable claim.
type asyncSnapshotWriter struct {
	storage snapshot.Storage
	logger  zerolog.Logger

	// ctx bounds the writer's whole life, not any one call. The inline path derived its timeout
	// from the tick context and cancelled it on return, which a goroutine outliving the call cannot
	// inherit: it would be cancelled the instant the tick's snapshot() returned. Per-write timeouts
	// hang off this instead, so a hung backend is still bounded without the writer's lifetime being
	// tied to a tick's.
	ctx    context.Context //nolint:containedctx // The writer's lifetime, not a request's; see above.
	cancel context.CancelFunc

	wake chan struct{} // Buffered 1. A token means "pending may be set"; extra tokens are harmless.
	done chan struct{} // Closed when the upload loop returns.

	mu         sync.Mutex
	pending    *cardinalv1.Snapshot // Newest snapshot not yet handed to storage; nil when there is none.
	pendingSeq uint64               // Sequence number of pending.
	submitted  uint64               // Sequence number of the newest snapshot ever handed to write.
	completed  uint64               // Sequence number of the newest snapshot Store has returned for.
	dropped    uint64               // Snapshots replaced while pending, so never written.
	progress   chan struct{}        // Closed and replaced whenever completed or stopped changes.
	started    bool                 // Whether the upload loop goroutine exists yet.
	stopped    bool                 // Whether stop() ran, or the loop gave up.
}

var _ snapshotWriter = (*asyncSnapshotWriter)(nil)

func newAsyncSnapshotWriter(storage snapshot.Storage, logger zerolog.Logger) *asyncSnapshotWriter {
	ctx, cancel := context.WithCancel(context.Background())
	return &asyncSnapshotWriter{
		storage:  storage,
		logger:   logger,
		ctx:      ctx,
		cancel:   cancel,
		wake:     make(chan struct{}, 1),
		done:     make(chan struct{}),
		progress: make(chan struct{}),
	}
}

// write publishes a snapshot to the upload loop, replacing any snapshot still waiting.
//
// This is the only part of the snapshot path that runs on the tick goroutine: a mutex, a pointer
// store and a non-blocking channel send. It never touches storage and never waits on the loop.
func (w *asyncSnapshotWriter) write(snap *cardinalv1.Snapshot) {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		// Only reachable after shutdown drained and stopped the writer, i.e. from a tick that
		// should no longer be running. Loud, because a silently discarded snapshot here would look
		// exactly like a working one.
		w.logger.Warn().Uint64("tick_height", snap.GetTickHeight()).
			Msg("dropping snapshot: the snapshot writer is stopped")
		return
	}
	w.submitted++
	superseded := w.pending
	if superseded != nil {
		w.dropped++
	}
	w.pending = snap
	w.pendingSeq = w.submitted
	dropped := w.dropped
	w.startLocked()
	w.mu.Unlock()

	// Wake the loop. A full buffer means a token nobody has consumed yet, which carries the same
	// message — re-read pending — so dropping this one loses nothing.
	select {
	case w.wake <- struct{}{}:
	default:
	}

	if superseded != nil {
		w.logger.Debug().
			Uint64("superseded_tick_height", superseded.GetTickHeight()).
			Uint64("tick_height", snap.GetTickHeight()).
			Uint64("dropped_total", dropped).
			Msg("superseded a pending snapshot: storage is slower than the snapshot rate")
	}
}

// startLocked launches the upload loop on first use. Worlds are built by NewWorld well before — and
// sometimes without ever — ticking, so the goroutine appears with the first snapshot rather than
// with the World, and a world that never snapshots never spawns one.
func (w *asyncSnapshotWriter) startLocked() {
	if w.started {
		return
	}
	w.started = true
	go w.loop()
}

func (w *asyncSnapshotWriter) loop() {
	defer close(w.done)
	for {
		select {
		case <-w.wake:
			w.flush()
		case <-w.ctx.Done():
			return
		}
	}
}

// flush writes pending snapshots until none is left. Taking pending and clearing it under the lock
// is what makes the write single-flight: a snapshot arriving mid-Store lands in the slot this loop
// will read next, replacing whatever else was waiting there.
func (w *asyncSnapshotWriter) flush() {
	for {
		w.mu.Lock()
		snap, seq := w.pending, w.pendingSeq
		w.pending = nil
		w.mu.Unlock()
		if snap == nil {
			return
		}

		writeCtx, cancel := context.WithTimeout(w.ctx, snapshotWriteTimeout)
		storeSnapshot(writeCtx, w.storage, snap, &w.logger)
		cancel()

		w.mu.Lock()
		w.completed = seq
		w.signalLocked()
		w.mu.Unlock()
	}
}

// signalLocked releases everyone waiting in drain so they can re-read the counters. Closing and
// replacing the channel is a broadcast that composes with a context deadline, which is why drain
// can be bounded by the shutdown budget and a sync.Cond could not.
func (w *asyncSnapshotWriter) signalLocked() {
	close(w.progress)
	w.progress = make(chan struct{})
}

// drain waits for every snapshot submitted before the call to have reached storage — or to have
// been superseded by a later one that did, which is the same thing to a reader of storage.
//
// Shutdown calls this right after taking the final snapshot, before NATS teardown, so that the last
// snapshot of a run is not the one the async path loses.
func (w *asyncSnapshotWriter) drain(ctx context.Context) error {
	w.mu.Lock()
	target := w.submitted
	for {
		if w.completed >= target {
			w.mu.Unlock()
			return nil
		}
		if w.stopped {
			outstanding := target - w.completed
			w.mu.Unlock()
			return eris.Errorf("snapshot writer stopped with %d write(s) outstanding", outstanding)
		}
		progress := w.progress
		w.mu.Unlock()

		select {
		case <-progress:
		case <-ctx.Done():
			w.mu.Lock()
			reached := w.completed >= target
			w.mu.Unlock()
			if reached {
				return nil
			}
			return eris.Wrap(ctx.Err(), "timed out waiting for pending snapshot writes to reach storage")
		}
		w.mu.Lock()
	}
}

// stop shuts the upload loop down and releases any drain waiting on writes that will now never
// happen. Cancelling the writer context also interrupts an in-flight Store, so a backend that
// respects its context lets stop return promptly; one that does not is abandoned when ctx expires
// rather than being allowed to hold shutdown open.
func (w *asyncSnapshotWriter) stop(ctx context.Context) {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	started := w.started
	w.signalLocked()
	w.mu.Unlock()

	w.cancel()
	if !started {
		return
	}
	select {
	case <-w.done:
	case <-ctx.Done():
		w.logger.Warn().Msg("snapshot writer did not stop within the shutdown budget; abandoning it")
	}
}

// droppedCount reports how many snapshots were superseded while pending, i.e. never written.
func (w *asyncSnapshotWriter) droppedCount() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dropped
}

// -------------------------------------------------------------------------------------------------
// World wiring
// -------------------------------------------------------------------------------------------------

// useInlineSnapshotStorage points both sides of the snapshot path at store and writes to it inline,
// on the caller's goroutine. It is how DST and the tests that assert on storage immediately after a
// snapshot opt out of the background writer; see inlineSnapshotWriter for why they must.
func (w *World) useInlineSnapshotStorage(store snapshot.Storage) {
	if w.snapshotWriter != nil {
		w.snapshotWriter.stop(context.Background())
	}
	w.snapshotStorage = store
	w.snapshotWriter = newInlineSnapshotWriter(store, w.tel.GetLogger("snapshot"))
}

// drainSnapshotWrites waits for the snapshots already handed to the writer — the final one above
// all — to reach storage, then shuts the writer down. Both steps share the shutdown budget, so a
// backend that has stopped answering delays shutdown by at most that budget and says so instead of
// hanging.
func (w *World) drainSnapshotWrites(ctx context.Context) {
	if err := w.snapshotWriter.drain(ctx); err != nil {
		// The final snapshot is the one this is most likely to be about, so name the consequence.
		w.tel.Logger.Error().Err(err).
			Msg("snapshot writes did not finish before shutdown; the last snapshot of this run may be lost")
	}
	w.snapshotWriter.stop(ctx)
}
