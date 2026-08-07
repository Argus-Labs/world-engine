package cardinal

import (
	"bytes"
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// -------------------------------------------------------------------------------------------------
// Test fixtures
// -------------------------------------------------------------------------------------------------

// syncBuffer is a log sink both the test goroutine and the writer goroutine may use. A
// strings.Builder cannot be: the writer logs every attempt, so the sink is shared by construction.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// blockingStorage stands in for a slow backend. Every Store announces itself on entered and then
// waits for a token on release, so a test decides exactly how long an upload takes. It records the
// tick height of every snapshot that completes, in completion order, which is what the ordering
// guarantee is stated in terms of: storage must never end up holding an older snapshot than one it
// already accepted.
//
// It asserts nothing, so unlike memSnapshotStorage it is safe to drive from the writer's goroutine.
type blockingStorage struct {
	entered chan uint64   // Tick height of a Store that has started.
	release chan struct{} // One token per Store that may finish.

	mu     sync.Mutex
	landed []uint64 // Tick heights that reached storage, in completion order.
}

var _ snapshot.Storage = (*blockingStorage)(nil)

func newBlockingStorage() *blockingStorage {
	return &blockingStorage{
		entered: make(chan uint64, 16),
		release: make(chan struct{}, 16),
	}
}

func (s *blockingStorage) Store(ctx context.Context, snap *cardinalv1.Snapshot) error {
	s.entered <- snap.GetTickHeight()
	select {
	case <-s.release:
	case <-ctx.Done():
		return ctx.Err()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.landed = append(s.landed, snap.GetTickHeight())
	return nil
}

func (s *blockingStorage) Load(_ context.Context) (*cardinalv1.Snapshot, error) {
	return nil, snapshot.ErrSnapshotNotFound
}

func (s *blockingStorage) landedTicks() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.landed...)
}

// waitEntered blocks until a Store has started, and reports the tick height it is writing.
func (s *blockingStorage) waitEntered(t *testing.T) uint64 {
	t.Helper()
	select {
	case height := <-s.entered:
		return height
	case <-time.After(5 * time.Second):
		t.Fatal("no Store call started")
		return 0
	}
}

// newTickableWorld builds a real World the way a game would — a hand-assembled one has no command
// or event manager and cannot be ticked — with the fixture components registered and a snapshot on
// every tick.
func newTickableWorld(t *testing.T) (*World, *snapshotEntities) {
	t.Helper()

	debug := false
	w, err := NewWorld(WorldOptions{
		Region:              "snapshot-writer",
		Organization:        "snapshot-writer",
		Project:             "snapshot-writer",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1, // Every tick snapshots, so a single Tick exercises the writer.
		Debug:               &debug,
	})
	require.NoError(t, err)

	state := &snapshotEntities{}
	require.NoError(t, initSystemFields(state, w))
	w.world.Init()
	return w, state
}

// newAsyncTestWorld builds a tickable World writing to store through the background writer StartGame
// installs, with logs captured so drop and drain reporting is assertable.
func newAsyncTestWorld(t *testing.T, store snapshot.Storage) (*World, *snapshotEntities, *syncBuffer) {
	t.Helper()

	w, state := newTickableWorld(t)

	logs := &syncBuffer{}
	logger := zerolog.New(logs).Level(zerolog.DebugLevel)
	w.tel.Logger = logger
	writer := newAsyncSnapshotWriter(store, logger, &w.tel)
	w.snapshotStorage = store
	w.snapshotWriter = writer
	t.Cleanup(func() { writer.stop(context.Background()) })

	return w, state, logs
}

// -------------------------------------------------------------------------------------------------
// Tests
// -------------------------------------------------------------------------------------------------

// TestSnapshotWriteDoesNotStallTick is the whole point of the change: a snapshot tick must not wait
// on the backend. The storage here never finishes on its own, so an inline write would park the
// tick goroutine until the 2s write timeout — with async it costs a mutex.
func TestSnapshotWriteDoesNotStallTick(t *testing.T) {
	t.Parallel()

	store := newBlockingStorage()
	w, state, _ := newAsyncTestWorld(t, store)
	seedSnapshotWorld(t, state)

	ticked := make(chan struct{})
	go func() {
		defer close(ticked)
		w.Tick(context.Background(), time.Now())
	}()

	// The upload must have started — otherwise this test would pass just as well if snapshots were
	// never written at all — and the tick must return while it is still blocked in Store.
	// The deadline is deliberately shorter than snapshotWriteTimeout: an inline write against this
	// storage would still return once its write context expired, so a longer wait would let the
	// tick "pass" by stalling for the full write budget — which is the very thing under test.
	store.waitEntered(t)
	select {
	case <-ticked:
	case <-time.After(snapshotWriteTimeout / 2):
		t.Fatal("Tick did not return while a snapshot write was in flight")
	}
	assert.Empty(t, store.landedTicks(), "the write completed, so the tick could not have been racing it")

	// Let the blocked write finish so the writer goroutine is not left parked.
	store.release <- struct{}{}
	require.NoError(t, w.snapshotWriter.drain(t.Context()))
	assert.Equal(t, []uint64{0}, store.landedTicks())
}

// TestSnapshotWriterLatestWins pins the ordering and memory rules of the single-flight writer:
// while one upload is in flight, a newer snapshot REPLACES the pending one rather than queueing
// behind it, so storage sees A then C — never B, and never anything older after something newer.
func TestSnapshotWriterLatestWins(t *testing.T) {
	t.Parallel()

	store := newBlockingStorage()
	logs := &syncBuffer{}
	// Info, i.e. the level a deployed shard runs at: the drop report has to survive it.
	writer := newAsyncSnapshotWriter(store, zerolog.New(logs).Level(zerolog.InfoLevel), nil)
	t.Cleanup(func() { writer.stop(context.Background()) })

	const (
		tickA uint64 = 1
		tickB uint64 = 2
		tickC uint64 = 3
	)

	// A goes in first and occupies the writer.
	writer.write(&cardinalv1.Snapshot{TickHeight: tickA, WorldState: &cardinalv1.WorldState{}})
	require.Equal(t, tickA, store.waitEntered(t), "the first snapshot must be the first one written")

	// B and C pile up behind it. B never reaches storage: C replaces it while it waits.
	writer.write(&cardinalv1.Snapshot{TickHeight: tickB, WorldState: &cardinalv1.WorldState{}})
	writer.write(&cardinalv1.Snapshot{TickHeight: tickC, WorldState: &cardinalv1.WorldState{}})
	assert.Equal(t, uint64(1), writer.droppedCount(), "the superseded snapshot must be counted")

	store.release <- struct{}{} // Finish A.
	require.Equal(t, tickC, store.waitEntered(t), "the writer must skip to the newest pending snapshot")
	store.release <- struct{}{} // Finish C.

	require.NoError(t, writer.drain(t.Context()))
	assert.Equal(t, []uint64{tickA, tickC}, store.landedTicks(),
		"storage must hold C last, must never see B, and must never take an older snapshot after a newer one")
	assert.Contains(t, logs.String(), "superseded a pending snapshot",
		"a dropped snapshot must be reported, or stale snapshots are unfalsifiable")
	assert.Contains(t, logs.String(), `"level":"warn"`,
		"the drop must be reported above the default log level, or an operator sees nothing")

	// stop reports the run's total, which is the only line guaranteed to carry the full count once
	// the per-drop lines are rate limited.
	writer.stop(context.Background())
	assert.Contains(t, logs.String(), "snapshots were superseded before they could be written during this run")
}

// TestSnapshotWriterDropLogIsRateLimited is the other half of drop reporting: a backend slower than
// the snapshot rate drops one snapshot per snapshot tick, so the report must not become a log line
// per tick. The first drop is reported and the next ones are counted silently until the Nth.
func TestSnapshotWriterDropLogIsRateLimited(t *testing.T) {
	t.Parallel()

	store := newBlockingStorage()
	logs := &syncBuffer{}
	writer := newAsyncSnapshotWriter(store, zerolog.New(logs).Level(zerolog.InfoLevel), nil)
	t.Cleanup(func() { writer.stop(context.Background()) })

	// The first write occupies the writer; every later one supersedes the pending snapshot.
	writer.write(&cardinalv1.Snapshot{TickHeight: 0, WorldState: &cardinalv1.WorldState{}})
	store.waitEntered(t)
	// The first of these fills the empty pending slot; every later one supersedes it, so the drop
	// count is one less than the number of writes.
	const drops = snapshotDropLogEvery + 1
	for i := range uint64(drops + 1) {
		writer.write(&cardinalv1.Snapshot{TickHeight: i + 1, WorldState: &cardinalv1.WorldState{}})
	}

	require.Equal(t, uint64(drops), writer.droppedCount(), "every superseded snapshot must be counted")
	assert.Equal(t, 2, strings.Count(logs.String(), "superseded a pending snapshot"),
		"exactly the first drop and the %dth may be logged", snapshotDropLogEvery)

	store.release <- struct{}{}
}

// TestSnapshotDrainWaitsForFinalSnapshot covers the durability boundary of the async path. Shutdown
// takes the final snapshot and then drains, deliberately before NATS teardown, so the one snapshot
// that has no successor to make up for it is not the one the background writer loses.
func TestSnapshotDrainWaitsForFinalSnapshot(t *testing.T) {
	t.Parallel()

	store := newBlockingStorage()
	w, state, logs := newAsyncTestWorld(t, store)
	seedSnapshotWorld(t, state)
	w.currentTick.height = 41
	w.stateAuthoritative = true

	w.finalSnapshot()
	store.waitEntered(t)

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		w.drainSnapshotWrites(context.Background())
	}()

	// The drain must be waiting on the blocked write, not sailing past it.
	select {
	case <-drained:
		t.Fatal("shutdown drained before the final snapshot reached storage")
	case <-time.After(100 * time.Millisecond):
	}

	store.release <- struct{}{}
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not return after the final snapshot landed")
	}

	assert.Equal(t, []uint64{41}, store.landedTicks(), "the final snapshot must be written before shutdown returns")
	assert.NotContains(t, logs.String(), "did not finish before shutdown", "a successful drain must not report a loss")
}

// TestSnapshotDrainTimeoutLogsInsteadOfHanging is the other half of the drain contract: shutdown
// runs on a budget, so a backend that has stopped answering must cost that budget and a log line —
// not a shard that never exits.
func TestSnapshotDrainTimeoutLogsInsteadOfHanging(t *testing.T) {
	t.Parallel()

	store := newBlockingStorage()
	w, state, logs := newAsyncTestWorld(t, store)
	seedSnapshotWorld(t, state)
	w.currentTick.height = 7
	w.stateAuthoritative = true

	w.finalSnapshot()
	store.waitEntered(t)

	// A budget that is already nearly gone, standing in for a shutdown whose earlier steps ate it.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	returned := make(chan struct{})
	go func() {
		defer close(returned)
		w.drainSnapshotWrites(ctx)
	}()

	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown hung on a backend that never answered")
	}

	assert.Empty(t, store.landedTicks(), "nothing should have landed: the write was still blocked")
	assert.Contains(t, logs.String(), "the last snapshot of this run may be lost",
		"an operator must be told the final snapshot did not make it")

	// stop() cancelled the writer context, so the abandoned Store returns rather than leaking.
	store.release <- struct{}{}
}

// TestSnapshotWriterConcurrentWithDebugState is the race-detector case. One world-state graph is
// read by three goroutines at once — the tick building the next one, DebugService.GetState
// serializing the published envelope, and the background writer marshaling the same envelope into
// storage — which is exactly the sharing the async writer adds a third party to.
func TestSnapshotWriterConcurrentWithDebugState(t *testing.T) {
	t.Parallel()

	debug := true
	w, err := NewWorld(WorldOptions{
		Region:              "async-race",
		Organization:        "async-race",
		Project:             "async-race",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1, // Every tick snapshots, so the writer is never idle.
		Debug:               &debug,
	})
	require.NoError(t, err)
	require.NotNil(t, w.debug, "GetState is only mounted with debug on")

	// A storage that marshals, so the writer genuinely reads the shared graph rather than
	// discarding it the way NopStorage would.
	store := &marshalingStorage{}
	w.snapshotStorage = store
	w.snapshotWriter = newAsyncSnapshotWriter(store, zerolog.Nop(), &w.tel)

	state := &snapshotEntities{}
	require.NoError(t, initSystemFields(state, w))
	w.world.Init()
	seedSnapshotWorld(t, state)

	ctx := context.Background()
	const ticks = 200

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // Readers: the debug RPC serializing the published envelope.
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			resp, err := w.debug.GetState(ctx, connect.NewRequest(&cardinalv1.GetStateRequest{}))
			assert.NoError(t, err)
			_, err = proto.Marshal(resp.Msg.GetSnapshot())
			assert.NoError(t, err)
		}
	}()

	for range ticks {
		w.Tick(ctx, time.Now())
	}
	close(stop)
	wg.Wait()

	require.NoError(t, w.snapshotWriter.drain(t.Context()))
	w.snapshotWriter.stop(context.Background())
	assert.Positive(t, store.count(), "the writer never stored anything, so nothing was shared")
}

// marshalingStorage does what a real backend does to the caller's graph — serialize it — and
// nothing else. It exists so the race test has a reader on the writer's goroutine.
type marshalingStorage struct {
	mu     sync.Mutex
	stored int
}

var _ snapshot.Storage = (*marshalingStorage)(nil)

func (s *marshalingStorage) Store(_ context.Context, snap *cardinalv1.Snapshot) error {
	if _, err := proto.Marshal(snap); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stored++
	return nil
}

func (s *marshalingStorage) Load(_ context.Context) (*cardinalv1.Snapshot, error) {
	return nil, snapshot.ErrSnapshotNotFound
}

func (s *marshalingStorage) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stored
}

// -------------------------------------------------------------------------------------------------
// Writer lifecycle
// -------------------------------------------------------------------------------------------------

// TestHandTickedWorldOwnsNoGoroutine pins the writer's lifecycle rule. The async writer parks a
// goroutine that only World.shutdown stops, and only StartGame runs shutdown — so a World ticked by
// hand (every test, DST, plugin harness and embedder) must keep the inline writer. Give it the async
// one and each such world leaks a parked goroutine pinning the storage backend and up to one full
// world-state graph, for as long as the process lives.
func TestHandTickedWorldOwnsNoGoroutine(t *testing.T) {
	// Not parallel: it counts process-wide goroutines.
	baseline := runtime.NumGoroutine()

	const worlds = 8
	for range worlds {
		w, state := newTickableWorld(t)
		require.IsType(t, &inlineSnapshotWriter{}, w.snapshotWriter,
			"a World that NewWorld built and nobody started must write snapshots inline")
		seedSnapshotWorld(t, state)
		for range 3 { // SnapshotRate is 1, so every tick writes a snapshot.
			w.Tick(context.Background(), time.Now())
		}
		// Telemetry owns background goroutines of its own — an OTLP exporter and its grpc plumbing,
		// spawned by every NewWorld — which would otherwise be what this count measures. Shutting
		// telemetry down leaves the snapshot writer as the only thing that can still be running.
		// Note this is NOT World.shutdown: nothing here stops a snapshot writer.
		if err := w.tel.Shutdown(context.Background()); err != nil {
			t.Logf("telemetry shutdown: %v", err) // Unreachable collector; irrelevant to the count.
		}
	}

	got := waitForGoroutines(baseline)
	assert.LessOrEqual(t, got, baseline,
		"%d worlds ticked without StartGame leaked %d goroutine(s)", worlds, got-baseline)
}

// TestStartGameSwitchesToTheAsyncWriter is the other side of the same rule: the async path must
// still be what a real, started shard uses — the inline default is a lifecycle decision, not a
// silent revert of the change that took uploads off the tick goroutine.
func TestStartGameSwitchesToTheAsyncWriter(t *testing.T) {
	t.Parallel()

	w, state := newTickableWorld(t)
	seedSnapshotWorld(t, state)
	require.IsType(t, &inlineSnapshotWriter{}, w.snapshotWriter)

	// What StartGame does, minus the signal handling and the shard loop it would then run.
	w.useAsyncSnapshotWriter()
	writer, ok := w.snapshotWriter.(*asyncSnapshotWriter)
	require.True(t, ok, "StartGame must take snapshot uploads off the tick goroutine")

	w.Tick(context.Background(), time.Now()) // SnapshotRate is 1, so this starts the upload loop.
	require.True(t, writer.started)

	// And shutdown's drain step stops it again, which is the whole reason StartGame may own it.
	w.drainSnapshotWrites(context.Background())
	select {
	case <-writer.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the writer goroutine outlived the shutdown that is supposed to own it")
	}
}

// TestDSTUsesTheInlineWriter pins DST to inline writes. DST's premise is that a seed reproduces a
// run, and latest-wins makes the set of snapshots that survive depend on how an upload goroutine
// interleaves with the tick loop; its in-memory backend also asserts on a *testing.T, which only the
// test goroutine may do. This holds twice over now — inline is the default for a hand-ticked world
// AND the fixture asks for it — so the test bites if either half is undone.
func TestDSTUsesTheInlineWriter(t *testing.T) {
	t.Parallel()

	w, _ := newTickableWorld(t)
	w.useInlineSnapshotStorage(&memSnapshotStorage{t: t})
	assert.IsType(t, &inlineSnapshotWriter{}, w.snapshotWriter,
		"DST must never write snapshots from a background goroutine")
}

// waitForGoroutines waits for the process-wide goroutine count to settle at or below want, and
// reports the last count it saw. A goroutine on its way out needs a scheduler tick or two to
// disappear, so one sample would be noise — while a leaked goroutine never goes away at all.
func waitForGoroutines(want int) int {
	deadline := time.Now().Add(3 * time.Second)
	for {
		n := runtime.NumGoroutine()
		if n <= want || time.Now().After(deadline) {
			return n
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// -------------------------------------------------------------------------------------------------
// Failing and panicking backends
// -------------------------------------------------------------------------------------------------

// errStorageRefused is what failingStorage answers every write with.
var errStorageRefused = errors.New("storage refused the snapshot")

// failingStorage rejects every write, standing in for a backend that is up but says no — an expired
// credential, a bucket policy, a full stream.
type failingStorage struct{}

var _ snapshot.Storage = (*failingStorage)(nil)

func (*failingStorage) Store(context.Context, *cardinalv1.Snapshot) error { return errStorageRefused }

func (*failingStorage) Load(context.Context) (*cardinalv1.Snapshot, error) {
	return nil, snapshot.ErrSnapshotNotFound
}

// TestSnapshotDrainReportsStoreFailure is the accounting rule drain exists for: a snapshot storage
// refused never reached storage, so drain must say so — and must say it promptly, because a backend
// that fails every write has no retry coming that waiting could catch.
func TestSnapshotDrainReportsStoreFailure(t *testing.T) {
	t.Parallel()

	w, state, logs := newAsyncTestWorld(t, &failingStorage{})
	seedSnapshotWorld(t, state)
	w.currentTick.height = 9
	w.stateAuthoritative = true

	w.finalSnapshot()

	// An unbounded context on purpose: nothing but the accounting can end this call, so a drain
	// that reported the failed write as completed would hang here forever.
	drained := make(chan error, 1)
	go func() { drained <- w.snapshotWriter.drain(context.Background()) }()

	var err error
	select {
	case err = <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("drain hung on a backend that rejects every write")
	}
	require.Error(t, err, "a snapshot storage refused must never be reported as drained")
	require.ErrorIs(t, err, errStorageRefused, "the drain error must name the backend's failure")

	// And shutdown escalates it, which is the whole point of drain returning an error.
	w.drainSnapshotWrites(context.Background())
	assert.Contains(t, logs.String(), "the last snapshot of this run may be lost")
}

// panickingStorage panics on its first panics calls and stores after that, so a test can watch the
// writer both survive a panic and keep serving.
type panickingStorage struct {
	mu     sync.Mutex
	panics int
	landed []uint64
}

var _ snapshot.Storage = (*panickingStorage)(nil)

func (s *panickingStorage) Store(_ context.Context, snap *cardinalv1.Snapshot) error {
	s.mu.Lock()
	if s.panics > 0 {
		s.panics--
		s.mu.Unlock()
		panic("backend exploded")
	}
	defer s.mu.Unlock()
	s.landed = append(s.landed, snap.GetTickHeight())
	return nil
}

func (*panickingStorage) Load(context.Context) (*cardinalv1.Snapshot, error) {
	return nil, snapshot.ErrSnapshotNotFound
}

func (s *panickingStorage) landedTicks() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.landed...)
}

// TestSnapshotWriterSurvivesStoragePanic pins the writer's panic policy. Store runs on the writer's
// own goroutine, so an unrecovered panic there would kill the process without running shutdown —
// losing the final snapshot and every other cleanup step over one bad write. The panic is instead
// reported, counted as a failed write so drain does not wait on it, and the writer stays alive.
func TestSnapshotWriterSurvivesStoragePanic(t *testing.T) {
	t.Parallel()

	store := &panickingStorage{panics: 1}
	logs := &syncBuffer{}
	// tel is nil: Sentry is not initialized in tests, so the capture would be a no-op anyway and
	// the log line is what is assertable here.
	writer := newAsyncSnapshotWriter(store, zerolog.New(logs).Level(zerolog.InfoLevel), nil)
	t.Cleanup(func() { writer.stop(context.Background()) })

	writer.write(&cardinalv1.Snapshot{TickHeight: 1, WorldState: &cardinalv1.WorldState{}})
	err := writer.drain(t.Context())
	require.Error(t, err, "a write that panicked must be reported as a failure, not waited on")
	assert.Contains(t, err.Error(), "panicked")
	assert.Contains(t, logs.String(), "recovered a panic from snapshot storage")
	assert.Empty(t, store.landedTicks())

	// The writer is still serving: the next snapshot reaches storage, and drain reports it landed.
	writer.write(&cardinalv1.Snapshot{TickHeight: 2, WorldState: &cardinalv1.WorldState{}})
	require.NoError(t, writer.drain(t.Context()))
	assert.Equal(t, []uint64{2}, store.landedTicks(), "the writer died with the panic instead of surviving it")
}
