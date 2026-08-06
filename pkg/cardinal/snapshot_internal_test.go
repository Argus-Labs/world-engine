package cardinal

import (
	"context"
	"maps"
	"slices"
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
//
// Writes go inline, so a snapshot has reached storage by the time snapshot() returns and these
// tests can assert on what it holds. The background writer is covered separately, in
// snapshot_writer_internal_test.go.
func newSnapshotTestWorld(t *testing.T, store snapshot.Storage) (*World, *snapshotEntities) {
	t.Helper()

	w := &World{
		world: ecs.NewWorld(),
		tel:   telemetry.Telemetry{Logger: zerolog.Nop()},
	}
	w.useInlineSnapshotStorage(store)
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

	src.snapshot(timestamp, want)
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

// TestRestoredWorldStaysUsable drives a restored world instead of only comparing it.
//
// Since format version 2 the entity -> archetype and entity -> row tables are rebuilt on load
// rather than persisted, which leaves two things a proto comparison cannot see. ToProto no longer
// emits either table, so TestSnapshotRoundTripThroughStorage's proto.Equal would pass even if the
// rebuild produced garbage. And a rebuilt table is sized to the highest live entity, while the
// table it replaces grew to the high-water mark of every ID ever allocated — so the restored world
// starts with shorter backing arrays than the one it came from.
//
// The cover for both is to use the world after restoring it: read every survivor, then create,
// destroy, and snapshot again.
func TestRestoredWorldStaysUsable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	store := &memSnapshotStorage{t: t}

	src, srcState := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, srcState)
	src.currentTick.height = 7

	// The world as it stood before it was ever serialized. seedSnapshotWorld destroys one of the
	// entities it creates, so the tables being rebuilt carry a tombstone and the free list is
	// non-empty — the shape most likely to expose a bad rebuild.
	want := map[EntityID]Position3D{}
	for eid, e := range srcState.Entities.Iter() {
		want[eid] = e.Position.Get()
	}
	require.Len(t, want, 5, "fixture should leave five live entities")

	pb, err := src.world.ToProto()
	require.NoError(t, err)
	src.snapshot(time.Unix(1700000000, 0).UTC(), pb)

	dst, dstState := newSnapshotTestWorld(t, store)
	require.NoError(t, dst.restore(ctx))

	// Every survivor resolves through the rebuilt tables, by iteration and by ID. Iteration walks
	// archetype entity lists; GetByID goes through entityArch and rows, so the two paths agreeing
	// is what says the rebuild matches the data it was derived from.
	got := map[EntityID]Position3D{}
	for eid, e := range dstState.Entities.Iter() {
		got[eid] = e.Position.Get()
	}
	assert.Equal(t, want, got, "restored world iterates a different entity set")

	for eid, pos := range want {
		e, err := dstState.Entities.GetByID(eid)
		require.NoError(t, err, "entity %d unreachable after restore", eid)
		assert.Equal(t, pos, e.Position.Get(), "entity %d has the wrong position after restore", eid)
	}

	// A create must extend the shortened tables rather than write past them, and must not hand out
	// an ID that is already live.
	newID, newEntity := dstState.Entities.Create()
	newEntity.Position.Set(Position3D{X: 99})
	assert.NotContains(t, want, newID, "create reused a live entity ID")

	fetched, err := dstState.Entities.GetByID(newID)
	require.NoError(t, err, "entity created after restore is unreachable")
	assert.Equal(t, Position3D{X: 99}, fetched.Position.Get())

	// Destroying a restored entity swap-removes it from its archetype, which rewrites the row of
	// whichever entity was moved into the freed slot. Sorted so a failure names the same entity on
	// every run.
	doomed := slices.Sorted(maps.Keys(want))[0]
	require.True(t, dstState.Entities.Destroy(doomed))
	_, err = dstState.Entities.GetByID(doomed)
	require.Error(t, err, "destroyed entity still resolves after restore")

	for eid, pos := range want {
		if eid == doomed {
			continue
		}
		e, err := dstState.Entities.GetByID(eid)
		require.NoError(t, err, "entity %d was lost when %d was destroyed", eid, doomed)
		assert.Equal(t, pos, e.Position.Get(), "entity %d took the wrong row when %d was destroyed", eid, doomed)
	}

	// Finally, the mutated world has to survive being snapshotted and restored again. A rebuild
	// that is self-consistent but disagrees with the archetype entity lists can pass everything
	// above and still produce a second-generation snapshot that does not match.
	dst.currentTick.height = 11
	mutated, err := dst.world.ToProto()
	require.NoError(t, err)
	dst.snapshot(time.Unix(1700000001, 0).UTC(), mutated)

	third, thirdState := newSnapshotTestWorld(t, store)
	require.NoError(t, third.restore(ctx))

	reloaded, err := third.world.ToProto()
	require.NoError(t, err)
	assert.True(t, proto.Equal(mutated, reloaded), "second-generation restore differs from what was stored")

	final := map[EntityID]Position3D{}
	for eid, e := range thirdState.Entities.Iter() {
		final[eid] = e.Position.Get()
	}
	assert.Len(t, final, len(want), "entity count drifted across two restores")
	assert.NotContains(t, final, doomed, "destroyed entity came back through a second restore")
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
	src.snapshot(time.Unix(1700000000, 0).UTC(), worldState)
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
	w.finalSnapshot()
	w.drainSnapshotWrites(context.Background())
	return err
}

// TestShutdownKeepsSnapshotWhenRestoreFailed is the state-loss guard: shutdown runs on every exit
// path, including the one taken when the restore failed, and the world it would persist there is
// empty only because it was never loaded. Writing it would replace the good snapshot with nothing.
func TestShutdownKeepsSnapshotWhenRestoreFailed(t *testing.T) {
	t.Parallel()

	store := &memSnapshotStorage{t: t}
	src, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	src.currentTick.height = 9

	worldState, err := src.world.ToProto()
	require.NoError(t, err)
	src.snapshot(time.Unix(1700000000, 0).UTC(), worldState)
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

	store := &memSnapshotStorage{t: t}
	src, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	src.currentTick.height = 3

	worldState, err := src.world.ToProto()
	require.NoError(t, err)
	src.snapshot(time.Unix(1700000000, 0).UTC(), worldState)
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
	timestamp := time.Unix(1700000000, 0).UTC()

	store := &memSnapshotStorage{t: t}
	w, state := newSnapshotTestWorld(t, store)
	seedSnapshotWorld(t, state)
	w.currentTick.height = 3

	worldState, err := w.world.ToProto()
	require.NoError(t, err)

	w.snapshot(timestamp, worldState)
	first, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)

	w.snapshot(timestamp, worldState)
	second, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the same world must serialize to the same snapshot bytes")
}

// TestSnapshotHandsStorageTheLiveGraph pins the caller side of Storage.Store's ownership rule:
// snapshot() passes the very world-state graph the tick built, with no copy, so a backend that
// retained it would be holding memory the caller owns and shares with other readers.
//
// The mutation below is a PROBE, not a reproduction of anything cardinal does: the graph is frozen
// once built (ToProto allocates a fresh one per call and nothing writes into it afterwards), and
// editing it is simply how a test can tell a copy from a retained pointer.
//
// The copy it then asserts is memSnapshotStorage's own — the in-memory backend used by DST and by
// these tests, NOT a production one. The production backends satisfy the same rule by serializing
// inside Store; that is covered against a real backend in the snapshot package
// (TestS3StorageStoreOwnership).
func TestSnapshotHandsStorageTheLiveGraph(t *testing.T) {
	t.Parallel()

	// What Store is handed is the caller's own graph, pointer for pointer: no copy is made on the
	// way in, which is why the ownership rule exists at all.
	rec := &liveGraphRecorder{}
	w, state := newSnapshotTestWorld(t, rec)
	seedSnapshotWorld(t, state)
	w.currentTick.height = 3

	live, err := w.world.ToProto()
	require.NoError(t, err)
	w.snapshot(time.Unix(1700000000, 0).UTC(), live)
	assert.Same(t, live, rec.got, "snapshot() must hand storage the live graph, not a copy of it")

	store := &memSnapshotStorage{t: t}
	w.useInlineSnapshotStorage(store)

	worldState, err := w.world.ToProto()
	require.NoError(t, err)

	w.snapshot(time.Unix(1700000000, 0).UTC(), worldState)
	require.NotNil(t, store.snap)

	before, err := proto.MarshalOptions{Deterministic: true}.Marshal(store.snap)
	require.NoError(t, err)

	// Editing the graph the caller still owns must not reach what memSnapshotStorage kept.
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
	w.snapshot(time.Unix(0, 0), worldState)

	require.NoError(t, w.restore(ctx), "a missing snapshot is not an error")
	assert.Equal(t, uint64(11), w.currentTick.height, "restore must not move the tick height")
	assert.Nil(t, w.state.Load(), "restore must not publish state when there is no snapshot")
}
