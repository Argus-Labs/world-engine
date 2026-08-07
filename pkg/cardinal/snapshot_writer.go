package cardinal

import (
	"context"
	"sync"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// snapshotWriteTimeout bounds a single Store call, so a backend that never answers cannot hold the
// writer — and with it every later snapshot — forever. It is the same 2s the inline write path used
// before the upload moved off the tick goroutine; what changed is where the clock starts, not how
// long a write may take.
const snapshotWriteTimeout = 2 * time.Second

// snapshotDropLogEvery rate-limits the drop report. A backend slower than the snapshot rate drops a
// snapshot every snapshot tick, so an unconditional warn per drop would be a log line per tick — an
// operator problem of its own. The first drop is always reported (that is the transition worth
// paging on) and then every Nth, with the running total on every line so the gaps are visible.
const snapshotDropLogEvery = 100

// snapshotWriter takes a finished snapshot envelope from the tick goroutine and gets it to storage.
//
// The tick goroutine must never wait on a backend, so write() takes no context and returns no
// error: snapshot writes are best-effort by contract (see World.snapshot), and an error the caller
// cannot act on is a lie about who handles it. Failures are logged by the writer and nothing else
// happens — the world keeps ticking and the next snapshot retries. Shutdown, which has no next
// snapshot to retry with, learns about them through drain.
type snapshotWriter interface {
	// write hands over a snapshot envelope. It never blocks on storage.
	//
	// Ownership: the writer may hold the message past the call, so the caller must treat it as
	// frozen from here on. Cardinal satisfies that by construction — every envelope wraps a world
	// state ToProto just built out of fresh slices, and nothing mutates it afterwards.
	write(snap *cardinalv1.Snapshot)

	// drain reports whether the snapshots handed to write before this call reached storage.
	//
	// It returns nil once storage holds a snapshot at least as new as the newest one submitted —
	// which a later snapshot supersedes just as well, since a reader of storage cannot tell the
	// difference. It returns an error if that never happened: a Store that failed, a writer that
	// stopped with writes outstanding, or ctx expiring first. It never blocks past ctx.
	//
	// A failed Store settles the snapshot it was given rather than being retried, so drain against
	// a persistently failing backend returns the failure promptly instead of waiting for a retry
	// that is not coming.
	drain(ctx context.Context) error

	// stop shuts the writer down. Snapshots handed to write afterwards are dropped, so callers that
	// care about the last one drain first. Bounded by ctx.
	stop(ctx context.Context)
}

// storeSnapshot performs one write attempt, logs it, and reports whether the snapshot landed. The
// error goes to the writer's own bookkeeping, never to the tick goroutine: snapshot writes are
// best-effort, and the caller of a snapshot write has nothing to do with an error but log it. What
// the error IS used for is drain, which must not report success for a snapshot storage refused.
func storeSnapshot(
	ctx context.Context,
	storage snapshot.Storage,
	snap *cardinalv1.Snapshot,
	logger *zerolog.Logger,
) error {
	if err := storage.Store(ctx, snap); err != nil {
		logger.Warn().Err(err).Uint64("tick_height", snap.GetTickHeight()).Msg("failed to store snapshot")
		return err
	}
	logger.Debug().Uint64("tick_height", snap.GetTickHeight()).Msg("published snapshot")
	return nil
}

// -------------------------------------------------------------------------------------------------
// Inline writer
// -------------------------------------------------------------------------------------------------

// inlineSnapshotWriter writes on the caller's goroutine, which is what the snapshot path did before
// the upload moved off the tick loop. It is the DEFAULT for every World, and the only writer a World
// that is not run by StartGame ever has (see World.useAsyncSnapshotWriter). It is what the callers
// that need a write to have happened by the time write() returns get:
//
//   - DST (see World.useInlineSnapshotStorage). A background writer would make DST irreproducible in
//     two separate ways. Latest-wins means the set of snapshots that actually reach storage depends
//     on how the upload goroutine interleaves with the tick loop, so the same seed could store a
//     different snapshot from run to run — and DST's whole premise is that a seed reproduces a run.
//     On top of that the in-memory backend asserts on a *testing.T, which only the test goroutine
//     may do. Determinism is worth more to DST than the tick latency the async writer buys, and DST
//     ticks against an in-memory map where there is no latency to buy back.
//   - Unit tests that assert on what storage holds right after a snapshot.
//   - Embedders that drive Tick themselves. They own no shutdown, so they would have nothing to stop
//     a background goroutine with.
type inlineSnapshotWriter struct {
	storage snapshot.Storage
	logger  zerolog.Logger

	// mu guards lastErr only. write and drain are usually the same goroutine here, but nothing in
	// the interface promises that, and the field is one word of contention on a path that has just
	// done a network write.
	mu sync.Mutex
	// lastErr is the outcome of the newest write attempt: nil once one succeeds, so drain reports a
	// failure only while storage is actually behind. Same latest-wins accounting as the async
	// writer, for the same reason — a later snapshot that landed makes an earlier failure moot.
	lastErr error
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
	err := storeSnapshot(ctx, w.storage, snap, &w.logger)

	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastErr = err
}

// drain has nothing to wait for — write() already stored everything it was handed — but it still
// reports the outcome of the newest write, so a backend that refused the final snapshot is not
// reported to shutdown as a success.
func (w *inlineSnapshotWriter) drain(_ context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.lastErr == nil {
		return nil
	}
	return eris.Wrap(w.lastErr, "the newest snapshot did not reach storage")
}

// stop is a no-op: there is nothing running to stop.
func (w *inlineSnapshotWriter) stop(_ context.Context) {}

// -------------------------------------------------------------------------------------------------
// Async writer
// -------------------------------------------------------------------------------------------------

// asyncSnapshotWriter uploads snapshots on a goroutine of its own, so a snapshot tick costs the
// shard loop a mutex instead of a marshal plus a network round trip. That is the entire point: the
// snapshot tick is a tail-latency spike, and the upload is the unbounded part of it.
//
// OPT-IN, AND ONLY FOR A WORLD THAT WILL BE SHUT DOWN. The goroutine is only as safe as the stop
// that matches it, and the only place with a guaranteed stop is StartGame's deferred shutdown. So
// StartGame installs this writer and nothing else does (see World.useAsyncSnapshotWriter): a World
// that is ticked by hand — tests, DST, plugin harnesses, embedders — keeps the inline writer and has
// no goroutine to leak.
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
// — just possibly an older one than the inline path would have left. Every drop is counted; the
// first is logged at WARN and then every snapshotDropLogEvery-th, and stop reports the run's total
// at warn if there was one, so "my snapshots are stale" stays a falsifiable claim.
type asyncSnapshotWriter struct {
	storage snapshot.Storage
	logger  zerolog.Logger
	// tel reports a panicking backend to Sentry, the way every other goroutine in the process does.
	// May be nil in tests that do not care; every production writer has one.
	tel *telemetry.Telemetry

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
	// attempted is the newest sequence number Store has RETURNED for, whether it stored the
	// snapshot, refused it, or panicked. It is what drain waits on, so a backend that fails every
	// write still lets drain finish — the failure is reported through lastErr, not by hanging.
	attempted uint64
	// stored is the newest sequence number that actually reached storage. drain compares against
	// this one, so a write that failed can never be reported as a success.
	stored uint64
	// lastErr is why the newest attempt did not land. Cleared as soon as one does: a later snapshot
	// that reached storage makes an earlier failure invisible to any reader of storage.
	lastErr  error
	dropped  uint64        // Snapshots replaced while pending, so never written.
	progress chan struct{} // Closed and replaced whenever the counters or stopped change.
	started  bool          // Whether the upload loop goroutine exists yet.
	stopped  bool          // Whether stop() ran, or the loop gave up.
}

var _ snapshotWriter = (*asyncSnapshotWriter)(nil)

func newAsyncSnapshotWriter(
	storage snapshot.Storage,
	logger zerolog.Logger,
	tel *telemetry.Telemetry,
) *asyncSnapshotWriter {
	ctx, cancel := context.WithCancel(context.Background())
	return &asyncSnapshotWriter{
		storage:  storage,
		logger:   logger,
		tel:      tel,
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
		w.reportDrop(superseded, snap, dropped)
	}
}

// reportDrop makes a superseded snapshot visible to an operator at the default log level, rate
// limited to the first drop and then every snapshotDropLogEvery-th (see the constant). Warn, not
// debug: a drop means storage is falling behind the snapshot rate, which is a durability change
// nobody would ever go looking for at debug level.
func (w *asyncSnapshotWriter) reportDrop(superseded, replacement *cardinalv1.Snapshot, dropped uint64) {
	if dropped != 1 && dropped%snapshotDropLogEvery != 0 {
		return
	}
	w.logger.Warn().
		Uint64("superseded_tick_height", superseded.GetTickHeight()).
		Uint64("tick_height", replacement.GetTickHeight()).
		Uint64("dropped_total", dropped).
		Uint64("log_every", snapshotDropLogEvery).
		Msg("superseded a pending snapshot: storage is slower than the snapshot rate")
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
		w.store(snap, seq)
	}
}

// store performs one write attempt and settles the accounting drain reads — on every path out,
// including a panic inside the backend.
//
// A PANICKING BACKEND DOES NOT KILL THE WRITER. Store runs on this goroutine, so an unrecovered
// panic there would take the process down without running shutdown, losing the final snapshot and
// every other cleanup step — a strictly worse outcome than the failed write it started as. So the
// panic is reported to Sentry (as any other goroutine's would be) and turned into a failed write:
// the loop stays alive, later snapshots still reach storage, and drain sees the failure rather than
// waiting for a write that already unwound.
func (w *asyncSnapshotWriter) store(snap *cardinalv1.Snapshot, seq uint64) {
	var err error
	defer func() {
		if r := recover(); r != nil {
			err = eris.Errorf("snapshot storage panicked: %v", r)
			w.logger.Error().Err(err).Uint64("tick_height", snap.GetTickHeight()).
				Msg("recovered a panic from snapshot storage; the snapshot was not written")
			if w.tel != nil {
				w.tel.CaptureException(w.ctx, err)
			}
		}
		w.settle(seq, err)
	}()

	writeCtx, cancel := context.WithTimeout(w.ctx, snapshotWriteTimeout)
	defer cancel()
	err = storeSnapshot(writeCtx, w.storage, snap, &w.logger)
}

// settle records the outcome of one write attempt and releases everyone in drain.
func (w *asyncSnapshotWriter) settle(seq uint64, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.attempted = seq
	if err == nil {
		w.stored = seq
		w.lastErr = nil
	} else {
		w.lastErr = err
	}
	w.signalLocked()
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
// SEMANTICS, DELIBERATE: an error is what a failure means, not what a wait means.
//
//   - nil means storage holds a snapshot at least as new as the newest one submitted before the
//     call. That is the only case where the data landed.
//   - A Store that returned an error, or panicked, SETTLES that snapshot: it is not retried, so
//     this call returns the failure instead of waiting for a retry that never comes. A backend
//     rejecting every write therefore costs shutdown one write timeout, not the whole budget.
//   - Anything else that means "not landed" — the writer stopped with writes outstanding, or ctx
//     expiring first — is an error too, naming which it was.
//
// Shutdown calls this right after taking the final snapshot, before NATS teardown, so that the last
// snapshot of a run is not the one the async path loses.
func (w *asyncSnapshotWriter) drain(ctx context.Context) error {
	w.mu.Lock()
	target := w.submitted
	for {
		if w.stored >= target {
			w.mu.Unlock()
			return nil
		}
		if w.attempted >= target {
			// Every submitted snapshot has been attempted and storage is still behind, so the
			// newest attempt failed and lastErr says how.
			err := w.lastErr
			w.mu.Unlock()
			return wrapUnlandedSnapshot(err)
		}
		if w.stopped {
			outstanding := target - w.attempted
			w.mu.Unlock()
			return eris.Errorf("snapshot writer stopped with %d write(s) outstanding", outstanding)
		}
		progress := w.progress
		w.mu.Unlock()

		select {
		case <-progress:
		case <-ctx.Done():
			w.mu.Lock()
			reached := w.stored >= target
			w.mu.Unlock()
			if reached {
				return nil
			}
			return eris.Wrap(ctx.Err(), "timed out waiting for pending snapshot writes to reach storage")
		}
		w.mu.Lock()
	}
}

// wrapUnlandedSnapshot names the failure that kept the newest snapshot out of storage. err is the
// writer's recorded failure; the fallback covers the impossible case of it having been cleared, so
// that a drain reporting failure never returns nil.
func wrapUnlandedSnapshot(err error) error {
	if err == nil {
		err = eris.New("snapshot storage rejected the write")
	}
	return eris.Wrap(err, "the newest snapshot did not reach storage")
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
	dropped := w.dropped
	w.signalLocked()
	w.mu.Unlock()

	// The run's total, once, at a level an operator sees: the per-drop lines are rate limited, so
	// this is the only place the full count is guaranteed to appear.
	if dropped > 0 {
		w.logger.Warn().Uint64("dropped_total", dropped).
			Msg("snapshots were superseded before they could be written during this run")
	}

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

// droppedCount reports how many snapshots were superseded while pending, i.e. never written. The
// operator-facing report of the same number is the log line in reportDrop and the total in stop;
// this accessor exists for the tests that assert on the count itself.
func (w *asyncSnapshotWriter) droppedCount() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.dropped
}

// -------------------------------------------------------------------------------------------------
// World wiring
// -------------------------------------------------------------------------------------------------

// useAsyncSnapshotWriter moves snapshot uploads off the tick goroutine for the rest of this world's
// life.
//
// ONLY StartGame MAY CALL THIS, and it is why the async writer is opt-in rather than the default.
// The writer owns a goroutine that parks holding the storage backend and up to one full world-state
// graph, and the only thing that ever releases it is World.shutdown — which only StartGame
// guarantees to run. A World ticked by hand (tests, DST, plugin harnesses, embedders) has no such
// guarantee, so it keeps the inline writer NewWorld gave it and there is no goroutine to leak.
func (w *World) useAsyncSnapshotWriter() {
	w.snapshotWriter.stop(context.Background()) // No-op for the inline writer NewWorld installed.
	w.snapshotWriter = newAsyncSnapshotWriter(w.snapshotStorage, w.tel.GetLogger("snapshot"), &w.tel)
}

// useInlineSnapshotStorage points both sides of the snapshot path at store and writes to it inline,
// on the caller's goroutine. DST and the tests that assert on storage immediately after a snapshot
// use it to swap the backend; the inline writer it installs is also what NewWorld installs, so this
// only pins what a hand-ticked world already had. See inlineSnapshotWriter for why they need it.
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
