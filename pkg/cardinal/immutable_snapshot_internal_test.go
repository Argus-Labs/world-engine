package cardinal

import (
	"cmp"
	"context"
	"maps"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"

	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

// These tests run immutable Maps and Slices through the production snapshot path: a system edits
// them, the tick writes the snapshot, and a fresh world loads it with restore.

type snapCell struct {
	HP   int64
	Tags immutable.Slice[string]
}

// snapDeep nests a Map three levels deep, with a struct holding a Slice at the bottom, beside a Slice
// of Maps.
type snapDeep struct {
	Regions immutable.Map[string, immutable.Map[uint32, immutable.Map[cellID, snapCell]]]
	Log     immutable.Slice[immutable.Map[int8, string]]
}

func (snapDeep) Name() string { return "snap_deep" }

type cellID uint16

type snapRow struct {
	Deep snapDeep
}

// snapGrow keeps three entities alive and changes every level of their snapDeep each tick: new and
// updated entries, an emptied inner Map, a removed top-level key and Maps built from Go maps.
type snapGrow struct {
	paused bool
}

func (s *snapGrow) Run(w *World) {
	if s.paused {
		return
	}
	n := 0
	for range w.Contains[snapRow]().Iter() {
		n++
	}
	for ; n < 3; n++ {
		w.Create[snapRow]()
	}

	tick := w.TickHeight()
	var row cellID
	for e := range w.Contains[snapRow]().Iter() {
		row++
		d := e.Get[snapDeep]()
		region := "r" + strconv.FormatUint(tick%3, 10)
		zone := uint32(tick % 2)

		zones, _ := d.Regions.Get(region)
		cells, _ := zones.Get(zone)
		b := cells.Edit()
		c, _ := b.Get(row)
		c.HP += int64(tick) + 1
		c.Tags.Append("t" + strconv.FormatUint(tick, 10))
		b.Set(row, c)
		b.Set(cellID(1000+tick%5), snapCell{HP: -int64(tick)})
		zones = zones.With(zone, b.Done()).With(99, immutable.Map[cellID, snapCell]{})
		d.Regions = d.Regions.With(region, zones)
		if tick%5 == 4 {
			d.Regions = d.Regions.Without("r0")
		}

		d.Log.Append(immutable.MapFrom(maps.All(map[int8]string{int8(tick % 7): "x", -1: region})))
		e.Set(d)
	}
}

// snapPeek edits every field of every snapDeep in place and never calls Set, so none of it may reach
// the world or a snapshot.
type snapPeek struct{}

func (*snapPeek) Run(w *World) {
	for e := range w.Contains[snapRow]().Iter() {
		d := e.Get[snapDeep]()
		for region, zones := range d.Regions.All() {
			b := zones.Without(0).Edit()
			b.Set(77, immutable.Map[cellID, snapCell]{}.With(1, snapCell{HP: 1}))
			b.Delete(1)
			for zone, cells := range zones.All() {
				for id, c := range cells.All() {
					if c.Tags.Len() > 0 {
						c.Tags.Set(0, "peek")
						c.Tags.Reverse()
					}
					cells = cells.With(id, c)
				}
				b.Set(zone, cells)
			}
			d.Regions = d.Regions.With(region, b.Done())
		}
		d.Log.SortStableFunc(func(a, b immutable.Map[int8, string]) int { return cmp.Compare(b.Len(), a.Len()) })
		d.Log.Reverse()
		d.Log.DeleteFunc(func(m immutable.Map[int8, string]) bool { return !m.Has(0) })
	}
}

func newSnapshotWorld(t *testing.T, storage snapshot.Storage, systems ...System) *World {
	t.Helper()
	w := newTestWorld(t)
	if storage != nil {
		w.useSyncSnapshotStorage(storage)
	}
	w.RegisterComponent[snapDeep]()
	for _, s := range systems {
		w.RegisterSystem(s)
	}
	w.init()
	return w
}

func tickN(w *World, n int) {
	for range n {
		w.Tick(time.Unix(int64(w.TickHeight()), 0))
	}
}

// requireSameWorld checks the restored world against the original twice over: every snapDeep value
// read back through an entity, and the whole encoded world state byte for byte.
func requireSameWorld(t *testing.T, want, got *World) {
	t.Helper()
	ids := 0
	for e := range want.Contains[snapRow]().Iter() {
		ids++
		restored := got.Entity(e.ID())
		require.True(t, restored.Alive(), "entity %d is missing after restore", e.ID())
		require.Equal(t, e.Get[snapDeep](), restored.Get[snapDeep](), "entity %d", e.ID())
	}
	require.Equal(t, 3, ids, "the system created the entities this test compares")
	require.Equal(t, want.world.EncodeState(nil), got.world.EncodeState(nil), "encoded world state")
	require.Equal(t, want.TickHeight(), got.TickHeight())
}

func TestSnapshot_NestedImmutableCollectionsRestore(t *testing.T) {
	ctx := context.Background()
	original := newSnapshotWorld(t, nil, &snapGrow{})
	storage := original.snapshotStorage
	tickN(original, 12)

	// The data must actually be deep for the comparison to mean anything.
	var deepest int
	for e := range original.Contains[snapRow]().Iter() {
		for _, zones := range e.Get[snapDeep]().Regions.All() {
			for _, cells := range zones.All() {
				for _, c := range cells.All() {
					deepest = max(deepest, c.Tags.Len())
				}
			}
		}
	}
	require.Greater(t, deepest, 1, "the innermost Slices hold several entries")

	first := newSnapshotWorld(t, storage, &snapGrow{})
	require.NoError(t, first.restore(ctx))
	requireSameWorld(t, original, first)

	// Through a second time: the restored world keeps editing, snapshots, and is loaded again.
	tickN(first, 7)
	second := newSnapshotWorld(t, storage, &snapGrow{})
	require.NoError(t, second.restore(ctx))
	requireSameWorld(t, first, second)

	// A restored world behaves exactly like one that never stopped.
	tickN(original, 7)
	requireSameWorld(t, original, second)
}

func TestSnapshot_DeriveWithoutSetLeavesSnapshotUnchanged(t *testing.T) {
	grow := &snapGrow{}
	w := newSnapshotWorld(t, nil, grow, &snapPeek{})
	tickN(w, 10)

	grow.paused = true
	tickN(w, 1)
	before := w.world.EncodeState(nil)
	var want cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(before, &want))

	for range 5 {
		tickN(w, 1)
		assert.Equal(t, before, w.world.EncodeState(nil), "tick %d changed the world without Set", w.TickHeight())

		data, err := w.snapshotStorage.Load(context.Background())
		require.NoError(t, err)
		snap, err := snapshot.Decode(data)
		require.NoError(t, err)
		require.Equal(t, w.TickHeight()-1, snap.GetTickHeight(), "this tick wrote the stored snapshot")
		assert.True(t, proto.Equal(&want, snap.GetWorldState()), "the stored snapshot changed without Set")
	}
}
