package cardinal

import (
	"bytes"
	"context"
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

// newAsyncTestWorld builds a real World — a hand-assembled one has no command or event manager and
// cannot be ticked — writing to store through the background writer the production path uses, with
// logs captured so drop and drain reporting is assertable.
func newAsyncTestWorld(t *testing.T, store snapshot.Storage) (*World, *snapshotEntities, *syncBuffer) {
	t.Helper()

	debug := false
	w, err := NewWorld(WorldOptions{
		Region:              "async-writer",
		Organization:        "async-writer",
		Project:             "async-writer",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        1, // Every tick snapshots, so a single Tick exercises the writer.
		Debug:               &debug,
	})
	require.NoError(t, err)

	logs := &syncBuffer{}
	logger := zerolog.New(logs).Level(zerolog.DebugLevel)
	w.tel.Logger = logger
	writer := newAsyncSnapshotWriter(store, logger)
	w.snapshotStorage = store
	w.snapshotWriter = writer
	t.Cleanup(func() { writer.stop(context.Background()) })

	state := &snapshotEntities{}
	require.NoError(t, initSystemFields(state, w))
	w.world.Init()
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
	// Not parallel: the drop report is a Debug line, and the process-wide zerolog level — info, set
	// once by telemetry's init — would otherwise filter it out. Raising it only makes other tests
	// more verbose, never less.
	previousLevel := zerolog.GlobalLevel()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() { zerolog.SetGlobalLevel(previousLevel) })

	store := newBlockingStorage()
	logs := &syncBuffer{}
	writer := newAsyncSnapshotWriter(store, zerolog.New(logs).Level(zerolog.DebugLevel))
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
	w.snapshotWriter = newAsyncSnapshotWriter(store, zerolog.Nop())

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
