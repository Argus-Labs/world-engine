package physics2d_test

// Lifecycle tests for the chain-geometry orphan sweep: deleted when the last reference goes,
// only after being referenced once — staged and post-Reset geometry are never touched.

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// sweepLine is a minimal chain polyline for lifecycle tests.
func sweepLine() []physics.Vec2 {
	return []physics.Vec2{{X: 10, Y: 0}, {X: 3, Y: 0}, {X: -3, Y: 0}, {X: -10, Y: 0}}
}

// chainBody returns a static body with one chain shape referencing geoID.
func chainBody(geoID cardinal.EntityID) physics.PhysicsBody2D {
	return newRigid(physics.BodyTypeStatic, physics.ColliderShape{
		ShapeType:     physics.ShapeTypeStaticChain,
		ChainGeometry: geoID,
		CategoryBits:  0xFFFF,
		MaskBits:      0xFFFF,
	})
}

// countGeometries registers a PostUpdate system that writes the current number of
// ChainGeometry2D entities into out every tick.
func countGeometries(w *cardinal.World, out *int) {
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Geo chainGeoSpawn
	}) {
		n := 0
		for range state.Geo.Iter() {
			n++
		}
		*out = n
	}, cardinal.WithHook(cardinal.PostUpdate))
}

func TestGeometrySweep_LastReferenceGone(t *testing.T) {
	t.Parallel()
	w, _ := makeWorld(t, physics.Vec2{})

	const destroyTick = 5
	var bodyID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
		Geo   chainGeoSpawn
	}) {
		switch state.Tick() {
		case 0:
			geoID := spawnChainGeometry(&state.Geo, sweepLine())
			id, row := state.Spawn.Create()
			row.Tag.Set(harnessTag{Role: "wall"})
			row.T.Set(physics.Transform2D{})
			row.V.Set(physics.Velocity2D{})
			row.PB.Set(chainBody(geoID))
			bodyID = id
		case destroyTick:
			require.True(t, state.Spawn.Destroy(bodyID))
		}
	}, cardinal.WithHook(cardinal.Update))

	var geoCount int
	countGeometries(w, &geoCount)

	initCardinalECS(w)
	tickN(t, w, destroyTick) // ticks 0..4: body alive, geometry referenced
	require.Equal(t, 1, geoCount, "geometry must live while referenced")

	tickN(t, w, 2) // tick 5 destroys the body; tick 6's pipeline sweeps
	require.Equal(t, 0, geoCount, "geometry must be swept once its last reference is gone")
}

func TestGeometrySweep_StagedNeverSwept(t *testing.T) {
	t.Parallel()
	w, _ := makeWorld(t, physics.Vec2{})

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Geo chainGeoSpawn
	}) {
		if state.Tick() == 0 {
			spawnChainGeometry(&state.Geo, sweepLine())
		}
	}, cardinal.WithHook(cardinal.Update))

	var geoCount int
	countGeometries(w, &geoCount)

	initCardinalECS(w)
	tickN(t, w, 10)
	require.Equal(t, 1, geoCount, "never-referenced geometry must not be swept (no timers)")
}

func TestGeometrySweep_SharedSweptOnlyAtLastReference(t *testing.T) {
	t.Parallel()
	w, _ := makeWorld(t, physics.Vec2{})

	var first, second cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
		Geo   chainGeoSpawn
	}) {
		switch state.Tick() {
		case 0:
			geoID := spawnChainGeometry(&state.Geo, sweepLine())
			for _, role := range []string{"wall_a", "wall_b"} {
				id, row := state.Spawn.Create()
				row.Tag.Set(harnessTag{Role: role})
				row.T.Set(physics.Transform2D{})
				row.V.Set(physics.Velocity2D{})
				row.PB.Set(chainBody(geoID))
				if role == "wall_a" {
					first = id
				} else {
					second = id
				}
			}
		case 3:
			require.True(t, state.Spawn.Destroy(first))
		case 6:
			require.True(t, state.Spawn.Destroy(second))
		}
	}, cardinal.WithHook(cardinal.Update))

	var geoCount int
	countGeometries(w, &geoCount)

	initCardinalECS(w)
	tickN(t, w, 6) // first body gone since tick 3; second still references
	require.Equal(t, 1, geoCount, "shared geometry must survive while any reference remains")

	tickN(t, w, 2) // tick 6 destroys the second body; tick 7 sweeps
	require.Equal(t, 0, geoCount, "shared geometry must be swept when the last reference goes")
}

// TestGeometrySweep_ResetKeepsUnreferenced covers the err-toward-keeping rule: armed state is
// derived, so after Reset (the restore-shaped path) unreferenced geometry counts as staged and
// is never swept.
func TestGeometrySweep_ResetKeepsUnreferenced(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{})

	const destroyTick = 3
	var bodyID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
		Geo   chainGeoSpawn
	}) {
		switch state.Tick() {
		case 0:
			geoID := spawnChainGeometry(&state.Geo, sweepLine())
			id, row := state.Spawn.Create()
			row.Tag.Set(harnessTag{Role: "wall"})
			row.T.Set(physics.Transform2D{})
			row.V.Set(physics.Velocity2D{})
			row.PB.Set(chainBody(geoID))
			bodyID = id
		case destroyTick:
			require.True(t, state.Spawn.Destroy(bodyID))
		}
	}, cardinal.WithHook(cardinal.Update))

	var geoCount int
	countGeometries(w, &geoCount)

	initCardinalECS(w)
	// Ticks 0..3: the body dies in tick 3's Update, after that tick's pipeline already ran,
	// so the orphaned geometry has not been swept yet — the sweep would fire at tick 4.
	tickN(t, w, destroyTick+1)
	p.Reset() // armed state dropped before tick 4: the orphan now looks staged
	tickN(t, w, 5)
	require.Equal(t, 1, geoCount,
		"after Reset, unreferenced geometry must be kept (treated as staged, never swept)")
}
