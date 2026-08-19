package cardinal

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

// newRestoreTestWorld builds a world wired to the given in-memory snapshot storage, so multiple
// worlds can hand snapshots to each other through it.
func newRestoreTestWorld(t *testing.T, store *memSnapshotStorage) (*World, *snapshotEntities) {
	t.Helper()
	t.Setenv("LOG_LEVEL", "disabled")

	debug := false
	w, err := NewWorld(WorldOptions{
		Region:              "restore",
		Organization:        "restore",
		Project:             "restore",
		ShardID:             "0",
		TickRate:            60,
		SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate:        5,
		Debug:               &debug,
	})
	require.NoError(t, err)

	w.snapshotStorage = store
	w.snapshotWriter = snapshot.NewSyncWriter(store, w.tel.GetLogger("snapshot"))

	state := &snapshotEntities{}
	require.NoError(t, initSystemFields(state, w))
	w.world.Init()
	return w, state
}

// storeSnapshot serializes the world and writes it to storage at the given height.
func storeSnapshot(t *testing.T, w *World, height uint64) {
	t.Helper()
	worldState, err := w.world.ToProto()
	require.NoError(t, err)
	w.snapshotWriter.Write(&cardinalv1.Snapshot{
		TickHeight: height,
		Timestamp:  timestamppb.New(time.Unix(1_700_000_000+int64(height), 0).UTC()),
		WorldState: worldState,
		Version:    snapshot.CurrentVersion,
	})
}

// TestRestoredWorldStaysUsable drives a restored world instead of only comparing it.
//
// The flat snapshot format stores no runtime layout: archetypes, the entity -> archetype index and
// the entity -> row tables are all rebuilt on load. A proto comparison cannot see a bad rebuild —
// ToProto never emits those tables — so the cover is to use the world after restoring it: read
// every survivor, then create, destroy, and snapshot again.
func TestRestoredWorldStaysUsable(t *testing.T) {
	ctx := context.Background()

	store := &memSnapshotStorage{t: t}

	src, srcState := newRestoreTestWorld(t, store)
	seedSnapshotWorld(t, srcState)

	// The world as it stood before it was ever serialized. seedSnapshotWorld destroys one of the
	// entities it creates, so the rebuilt tables carry a hole and the free list is non-empty — the
	// shape most likely to expose a bad rebuild.
	want := map[EntityID]Position3D{}
	for eid, e := range srcState.Entities.Iter() {
		want[eid] = e.Position.Get()
	}
	require.Len(t, want, 5, "fixture should leave five live entities")

	storeSnapshot(t, src, 7)

	dst, dstState := newRestoreTestWorld(t, store)
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

	// A create must extend the rebuilt tables rather than write past them, and must not hand out
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
	mutated, err := dst.world.ToProto()
	require.NoError(t, err)
	storeSnapshot(t, dst, 11)

	third, thirdState := newRestoreTestWorld(t, store)
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
