package physics2d_test

import (
	"testing"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal"
	phycomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
	"github.com/stretchr/testify/require"
)

// legacySingletonRow is a singleton carrying only the tag and a contact baseline, with no
// ShapeStore: the shape a snapshot written before the store existed restores.
type legacySingletonRow = cardinal.Exact[struct {
	Tag      cardinal.WithComponent[phycomp.PhysicsSingletonTag]
	Contacts cardinal.WithComponent[phycomp.ActiveContacts]
}]

type singletonMigrationState struct {
	cardinal.BaseSystemState
	Legacy    legacySingletonRow
	Singleton internal.SingletonSearch
}

// A singleton that lacks a component must be adopted and completed, not duplicated. Creating a
// second one beside it strands the contact baseline the old one holds, so every contact that
// was already live replays as a fresh Begin, which is the whole thing the baseline prevents.
//
// The pre-store singleton is created directly here; no snapshot is written or restored. What
// is under test is EnsureSingleton's handling of such an entity, whatever put it there.
func TestEnsureSingletonAdoptsAPreStoreSingleton(t *testing.T) {
	t.Parallel()
	debug := true
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		Region: "local", Organization: "wb-test", Project: "wb-test", ShardID: "0",
		TickRate: 60, SnapshotStorageType: snapshot.StorageTypeNop,
		SnapshotRate: 1_000_000, Debug: &debug,
	})
	require.NoError(t, err)
	// What Plugin.Register does; the plugin itself is not needed to drive EnsureSingleton.
	w.RegisterArchetype[internal.SingletonRow]()

	baseline := phycomp.ActiveContacts{Pairs: immutable.SliceOf(phycomp.ContactPairEntry{
		EntityA: 11, ShapeIndexA: 0, EntityB: 12, ShapeIndexB: 1,
	})}
	var legacy cardinal.EntityID
	w.RegisterSystem(func(state *singletonMigrationState) {
		if state.Tick() != 0 {
			return
		}
		row := state.Legacy.Create()
		row.Set(baseline)
		legacy = row.ID()
	}, cardinal.WithHook(cardinal.Init))

	var adopted cardinal.EntityID
	var tagged int
	var kept phycomp.ActiveContacts
	w.RegisterSystem(func(state *singletonMigrationState) {
		row := internal.EnsureSingleton(&state.Singleton)
		adopted, kept = row.ID(), row.Get[phycomp.ActiveContacts]()
		tagged = 0
		for range state.Singleton.Iter() {
			tagged++
		}
	}, cardinal.WithHook(cardinal.PreUpdate))

	initCardinalECS(w)
	w.Tick(time.Unix(1, 0))

	require.Equal(t, 1, tagged, "the pre-store singleton was duplicated instead of adopted")
	require.Equal(t, legacy, adopted, "a second singleton was created beside the pre-store one")
	require.Equal(t, baseline, kept, "the pre-store singleton's contact baseline was lost")
}
