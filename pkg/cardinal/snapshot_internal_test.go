package cardinal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

// snapshotEntities reuses the benchmark component fixtures (gob wire codecs) so the round-trip
// covers a scalar, an int pair and a slice-bearing component.
type snapshotEntities struct {
	Entities Contains[struct {
		Position  Ref[Position3D]
		Health    Ref[Health2]
		Inventory Ref[Inventory]
	}]
}

// newSnapshotTestWorld builds the smallest World that snapshot() and restore() need: an ECS world
// with the fixture components registered, a storage backend, and a silent logger.
func newSnapshotTestWorld(t *testing.T, store snapshot.Storage) (*World, *snapshotEntities) {
	t.Helper()

	w := &World{
		world:           ecs.NewWorld(),
		snapshotStorage: store,
		tel:             telemetry.Telemetry{Logger: zerolog.Nop()},
	}
	state := &snapshotEntities{}
	require.NoError(t, initSystemFields(state, w))
	return w, state
}

// seedSnapshotWorld creates a handful of entities, including one that is removed so the sparse sets
// carry tombstones and the free list is non-empty.
func seedSnapshotWorld(t *testing.T, state *snapshotEntities) {
	t.Helper()

	for i := range 5 {
		_, e := state.Entities.Create()
		e.Position.Set(Position3D{X: float64(i), Y: float64(i) * 2, Z: -1})
		e.Health.Set(Health2{Current: 100 - i, Max: 100})
		e.Inventory.Set(Inventory{Items: []string{"sword", "potion"}, Capacity: 10 + i})
	}
	doomed, e := state.Entities.Create()
	e.Position.Set(Position3D{X: 42})
	require.True(t, state.Entities.Destroy(doomed))
}

// liveGraphRecorder keeps the world-state pointer it was handed, so a test can assert what
// snapshot() passes down. Only a test may do this: Storage.Store's ownership rule forbids a real
// backend from holding the caller's message past the call.
type liveGraphRecorder struct {
	got *cardinalv1.WorldState
}

var _ snapshot.Storage = (*liveGraphRecorder)(nil)

func (r *liveGraphRecorder) Store(_ context.Context, snap *cardinalv1.Snapshot) error {
	r.got = snap.GetWorldState()
	return nil
}

func (r *liveGraphRecorder) Load(_ context.Context) (*cardinalv1.Snapshot, error) {
	return nil, snapshot.ErrSnapshotNotFound
}

// TestSnapshotRoundTripThroughStorage is the end-to-end contract for the snapshot path:
// ToProto -> Store -> Load -> FromProto -> ToProto must reproduce the same world state.
func TestSnapshotRoundTripThroughStorage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	timestamp := time.Unix(1700000000, 0).UTC()

	store := &memSnapshotStorage{t: t}

	src, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	src.currentTick.height = 7

	want, err := src.world.ToProto()
	require.NoError(t, err)
	// Guard against a vacuous roundtrip: the fixture must actually serialize component data.
	blobs := 0
	for _, arch := range want.GetArchetypes() {
		for _, col := range arch.GetColumns() {
			blobs += len(col.GetComponents())
		}
	}
	require.NotZero(t, blobs, "fixture world serialized no components")

	src.snapshot(ctx, timestamp, want)
	require.NotNil(t, store.snap, "snapshot() did not reach storage")
	assert.Equal(t, snapshot.CurrentVersion, store.snap.GetVersion())
	assert.Equal(t, uint64(7), store.snap.GetTickHeight())
	assert.Equal(t, timestamp, store.snap.GetTimestamp().AsTime())

	dst, _ := newSnapshotTestWorld(t, store)
	// The published state has exactly one reader, DebugService.GetState, so the publish is gated
	// on the debug module existing.
	dst.debug = newDebugModule(dst)
	require.NoError(t, dst.restore(ctx))

	got, err := dst.world.ToProto()
	require.NoError(t, err)
	assert.True(t, proto.Equal(want, got), "restored world state differs from the stored one")

	// Restoring resumes on the tick after the stored one, and republishes the loaded state.
	assert.Equal(t, uint64(8), dst.currentTick.height)
	published := dst.state.Load()
	require.NotNil(t, published)
	assert.Equal(t, uint64(7), published.GetTickHeight())
	assert.True(t, proto.Equal(want, published.GetWorldState()))

	// With debug off nothing can read the published state, and persistState never replaces it, so
	// restoring must not pin the restored graph for the life of the process.
	quiet, _ := newSnapshotTestWorld(t, store)
	require.NoError(t, quiet.restore(ctx))
	got, err = quiet.world.ToProto()
	require.NoError(t, err)
	assert.True(t, proto.Equal(want, got), "restore must rebuild the world whether or not debug is on")
	assert.Nil(t, quiet.state.Load(), "restore must not publish state when debug is off")
}

// TestRestoreChecksSnapshotVersion covers the guard that makes a future format change safe: a
// snapshot written by a build with a newer format is refused instead of being decoded into the
// wrong world, whichever Storage implementation returned it.
func TestRestoreChecksSnapshotVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &memSnapshotStorage{t: t}
	src, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	src.currentTick.height = 4

	worldState, err := src.world.ToProto()
	require.NoError(t, err)
	src.snapshot(ctx, time.Unix(1700000000, 0).UTC(), worldState)
	require.NotNil(t, store.snap)

	// The snapshot this build just wrote restores.
	dst, _ := newSnapshotTestWorld(t, store)
	require.NoError(t, dst.restore(ctx))
	assert.Equal(t, uint64(5), dst.currentTick.height)

	// The same snapshot stamped with a newer format does not.
	store.snap.Version = snapshot.CurrentVersion + 1
	stale, _ := newSnapshotTestWorld(t, store)
	before, err := stale.world.ToProto()
	require.NoError(t, err)

	err = stale.restore(ctx)
	require.Error(t, err)
	assert.True(t, eris.Is(err, snapshot.ErrUnsupportedVersion))
	assert.Contains(t, err.Error(), "refusing to restore snapshot")
	assert.Zero(t, stale.currentTick.height, "a refused snapshot must not move the tick height")

	after, err := stale.world.ToProto()
	require.NoError(t, err)
	assert.True(t, proto.Equal(before, after), "a refused snapshot must not be loaded into the world")
}

// runThenShutdown drives the two halves of a process lifetime that the final-snapshot guard sits
// between: run(), which restores and then loops until the context is done, and the final snapshot
// shutdown takes on every exit path. shutdown() itself is not driven here because its later steps
// need a live NATS service and pprof server; step 1 is called directly, exactly as shutdown calls
// it. The context is already cancelled, so a successful restore falls straight out of the loop.
func runThenShutdown(t *testing.T, w *World) error {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := w.run(ctx)
	w.finalSnapshot(context.Background())
	return err
}

// TestShutdownKeepsSnapshotWhenRestoreFailed is the state-loss guard: shutdown runs on every exit
// path, including the one taken when the restore failed, and the world it would persist there is
// empty only because it was never loaded. Writing it would replace the good snapshot with nothing.
func TestShutdownKeepsSnapshotWhenRestoreFailed(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &memSnapshotStorage{t: t}
	src, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	src.currentTick.height = 9

	worldState, err := src.world.ToProto()
	require.NoError(t, err)
	src.snapshot(ctx, time.Unix(1700000000, 0).UTC(), worldState)
	require.NotNil(t, store.snap)

	// Stamp the stored snapshot with a format this build refuses, so the restore fails the way a
	// corrupt envelope or a storage error would.
	store.snap.Version = snapshot.CurrentVersion + 1
	before, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)

	var logs strings.Builder
	w, _ := newSnapshotTestWorld(t, store)
	w.tel.Logger = zerolog.New(&logs)
	w.options.TickRate = 1

	err = runThenShutdown(t, w)
	require.Error(t, err, "an unreadable snapshot must stop the world")
	assert.True(t, eris.Is(err, snapshot.ErrUnsupportedVersion))

	after, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a failed restore must leave the persisted snapshot untouched")
	assert.Contains(t, logs.String(), "skipping final snapshot",
		"an operator must be told why no final snapshot was written")
}

// TestShutdownSnapshotsFreshWorld covers the other half of the guard: a world with nothing persisted
// yet is authoritative — the fresh world IS the state — so its first snapshot is the final one.
func TestShutdownSnapshotsFreshWorld(t *testing.T) {
	t.Parallel()

	store := &memSnapshotStorage{t: t}
	w, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	w.options.TickRate = 1
	require.Nil(t, store.snap, "storage must start empty for this case to mean anything")

	require.ErrorIs(t, runThenShutdown(t, w), context.Canceled)

	require.NotNil(t, store.snap, "a world that found no snapshot must still write one on shutdown")
	want, err := w.world.ToProto()
	require.NoError(t, err)
	assert.True(t, proto.Equal(want, store.snap.GetWorldState()),
		"the final snapshot must hold the world the run ended with")
}

// TestShutdownSnapshotsRestoredWorld is the ordinary lifetime: restore succeeded, so the world is
// the persisted state and shutdown writes it back.
func TestShutdownSnapshotsRestoredWorld(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &memSnapshotStorage{t: t}
	src, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	src.currentTick.height = 3

	worldState, err := src.world.ToProto()
	require.NoError(t, err)
	src.snapshot(ctx, time.Unix(1700000000, 0).UTC(), worldState)
	require.NotNil(t, store.snap)

	w, _ := newSnapshotTestWorld(t, store)
	w.options.TickRate = 1
	require.ErrorIs(t, runThenShutdown(t, w), context.Canceled)

	assert.Equal(t, uint64(4), store.snap.GetTickHeight(), "the final snapshot carries the resumed tick")
	assert.True(t, proto.Equal(worldState, store.snap.GetWorldState()),
		"the final snapshot must hold the restored world, not an empty one")
}

// TestSnapshotStorageBytesAreStable guards that the same world always reaches storage as the same
// bytes, whichever backend serializes it.
func TestSnapshotStorageBytesAreStable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	timestamp := time.Unix(1700000000, 0).UTC()

	store := &memSnapshotStorage{t: t}
	w, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	w.currentTick.height = 3

	worldState, err := w.world.ToProto()
	require.NoError(t, err)

	w.snapshot(ctx, timestamp, worldState)
	first, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)

	w.snapshot(ctx, timestamp, worldState)
	second, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the same world must serialize to the same snapshot bytes")
}

// TestSnapshotHandsStorageTheLiveGraph pins the caller side of Storage.Store's ownership rule:
// snapshot() passes the very world-state graph the tick built, with no copy, so a backend that
// retained it would store whatever a later tick made of it.
//
// The copy it then asserts is memSnapshotStorage's own — the in-memory backend used by DST and by
// these tests, NOT a production one. The production backends satisfy the same rule by serializing
// inside Store; that is covered against a real backend in the snapshot package
// (TestS3StorageStoreOwnership).
func TestSnapshotHandsStorageTheLiveGraph(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// What Store is handed is the caller's own graph, pointer for pointer: no copy is made on the
	// way in, which is why the ownership rule exists at all.
	rec := &liveGraphRecorder{}
	w, state := newSnapshotTestWorld(t, rec)
	seedSnapshotWorld(t, state)
	w.currentTick.height = 3

	live, err := w.world.ToProto()
	require.NoError(t, err)
	w.snapshot(ctx, time.Unix(1700000000, 0).UTC(), live)
	assert.Same(t, live, rec.got, "snapshot() must hand storage the live graph, not a copy of it")

	store := &memSnapshotStorage{t: t}
	w.snapshotStorage = store

	worldState, err := w.world.ToProto()
	require.NoError(t, err)

	w.snapshot(ctx, time.Unix(1700000000, 0).UTC(), worldState)
	require.NotNil(t, store.snap)

	before, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)

	// A later tick mutating the live graph must not reach what memSnapshotStorage kept.
	worldState.NextId += 1000
	after, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)
	assert.Equal(t, before, after, "memSnapshotStorage kept a reference to the caller's graph")
}

// TestSnapshotNopStorage covers the default storage type: storing succeeds and does nothing, and a
// restore finds no snapshot and leaves the world untouched.
func TestSnapshotNopStorage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	w, state := newSnapshotTestWorld(t, snapshot.NewNopStorage())
	seedSnapshotWorld(t, state)
	w.currentTick.height = 11

	worldState, err := w.world.ToProto()
	require.NoError(t, err)
	w.snapshot(ctx, time.Unix(0, 0), worldState)

	require.NoError(t, w.restore(ctx), "a missing snapshot is not an error")
	assert.Equal(t, uint64(11), w.currentTick.height, "restore must not move the tick height")
	assert.Nil(t, w.state.Load(), "restore must not publish state when there is no snapshot")
}
