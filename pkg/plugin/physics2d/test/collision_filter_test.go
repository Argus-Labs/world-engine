package physics2d_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Category/Mask filtering — bodies on different layers don't collide
// ---------------------------------------------------------------------------

func TestFilter_BodiesOnDifferentLayersDontCollide(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var ballID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Floor on layer 0x0001.
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "floor"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 20, Y: 0.5},
			CategoryBits: 0x0001,
			MaskBits:     0x0001, // Only collides with 0x0001
		}))

		// Ball on layer 0x0002 — should pass through the floor.
		id, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "ghost_ball"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 5}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			Density:      1,
			CategoryBits: 0x0002,
			MaskBits:     0x0002, // Only collides with 0x0002
		}))
		ballID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 120 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == ballID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 121)

	// Ball should have fallen through the floor (way below Y=0).
	require.Less(t, finalPos.Y, -5.0,
		"ball on different layer should pass through floor")
}

// ---------------------------------------------------------------------------
// Category/Mask filtering — bodies on same layer DO collide
// ---------------------------------------------------------------------------

func TestFilter_BodiesOnSameLayerCollide(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: -10})

	var ballID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Floor on layer 0x0001.
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "floor"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 20, Y: 0.5},
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}))

		// Ball on layer 0x0001 — should collide with the floor.
		id, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "solid_ball"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 5}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeDynamic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       0.5,
			Density:      1,
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}))
		ballID = id
	}, cardinal.WithHook(cardinal.Init))

	var finalPos physics.Vec2
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() < 120 {
			return
		}
		for eid, row := range state.Spawn.Iter() {
			if eid == ballID {
				finalPos = row.T.Get().Position
			}
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 121)

	// Ball should rest on the floor near Y ≈ 1 (floor top at 0.5 + ball radius 0.5).
	require.Greater(t, finalPos.Y, -0.5,
		"ball on same layer should rest on the floor, not fall through")
}

// ---------------------------------------------------------------------------
// Raycast filter — Category/Mask filtering applies to queries
// ---------------------------------------------------------------------------

func TestFilter_RaycastCategoryMask(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var wallAID, wallBID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Wall A on layer 0x0001 at X=5.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "wall_a"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}))
		wallAID = id

		// Wall B on layer 0x0002 at X=10.
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "wall_b"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 10, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0x0002,
			MaskBits:     0xFFFF,
		}))
		wallBID = id2
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Default filter (all layers) — should hit closest wall (A).
	rayAll := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 20, Y: 0},
	})
	require.True(t, rayAll.Hit)
	require.Equal(t, wallAID, rayAll.Entity, "default ray hits closest wall A")

	// Filter for layer 0x0002 only — should skip A and hit B.
	fl := physics.Filter{CategoryBits: 0x0002, MaskBits: 0x0002}
	rayFiltered := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 20, Y: 0},
		Filter: &fl,
	})
	require.True(t, rayFiltered.Hit)
	require.Equal(t, wallBID, rayFiltered.Entity, "filtered ray skips A and hits B")

	// Filter for layer 0x0004 (nothing) — should miss.
	flNone := physics.Filter{CategoryBits: 0x0004, MaskBits: 0x0004}
	rayNone := physics.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 20, Y: 0},
		Filter: &flNone,
	})
	require.False(t, rayNone.Hit, "filter with no matching layers hits nothing")
}

// ---------------------------------------------------------------------------
// AABB overlap — sensor excluded by default, included with IncludeSensors
// ---------------------------------------------------------------------------

func TestFilter_AABBSensorExcludedByDefault(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var sensorID, solidID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Solid box.
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "solid"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 1, Y: 1},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		solidID = id

		// Sensor circle at same position.
		id2, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "sensor"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 0, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeCircle,
			Radius:       1,
			IsSensor:     true,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		sensorID = id2
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Default (no filter / sensors excluded).
	ovDefault := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: -2, Y: -2},
		Max: physics.Vec2{X: 2, Y: 2},
	})
	foundSolid, foundSensor := false, false
	for _, h := range ovDefault.Hits {
		if h.Entity == solidID {
			foundSolid = true
		}
		if h.Entity == sensorID {
			foundSensor = true
		}
	}
	require.True(t, foundSolid, "solid found by default")
	require.False(t, foundSensor, "sensor excluded by default")

	// IncludeSensors = true.
	fl := physics.Filter{CategoryBits: 0xFFFF, MaskBits: 0xFFFF, IncludeSensors: true}
	ovInclude := physics.OverlapAABB(physics.AABBOverlapRequest{
		Min:    physics.Vec2{X: -2, Y: -2},
		Max:    physics.Vec2{X: 2, Y: 2},
		Filter: &fl,
	})
	foundSolid, foundSensor = false, false
	for _, h := range ovInclude.Hits {
		if h.Entity == solidID {
			foundSolid = true
		}
		if h.Entity == sensorID {
			foundSensor = true
		}
	}
	require.True(t, foundSolid, "solid found with IncludeSensors")
	require.True(t, foundSensor, "sensor found with IncludeSensors")
}

// ---------------------------------------------------------------------------
// CircleSweep filter — sensors excluded by default
// ---------------------------------------------------------------------------

func TestFilter_CircleSweepSensorExcluded(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var solidID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		// Sensor at X=5.
		_, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "sensor_wall"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			IsSensor:     true,
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))

		// Solid at X=10.
		id, row2 := state.Spawn.Create()
		row2.Tag.Set(harnessTag{Role: "solid_wall"})
		row2.T.Set(physics.Transform2D{Position: physics.Vec2{X: 10, Y: 0}})
		row2.V.Set(physics.Velocity2D{})
		row2.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0xFFFF,
			MaskBits:     0xFFFF,
		}))
		solidID = id
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)

	// Default sweep should skip sensor and hit solid.
	sweep := physics.CircleSweep(physics.CircleSweepRequest{
		Start:  physics.Vec2{X: 0, Y: 0},
		End:    physics.Vec2{X: 20, Y: 0},
		Radius: 0.2,
	})
	require.True(t, sweep.Hit)
	require.Equal(t, solidID, sweep.Entity, "sweep skips sensor, hits solid")
}

// ---------------------------------------------------------------------------
// Filter changes mid-sim — changing CategoryBits should affect queries
// ---------------------------------------------------------------------------

func TestFilter_ChangeCategoryBitsMidSim(t *testing.T) {
	w := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	var wallID cardinal.EntityID

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Spawn.Create()
		row.Tag.Set(harnessTag{Role: "switchable_wall"})
		row.T.Set(physics.Transform2D{Position: physics.Vec2{X: 5, Y: 0}})
		row.V.Set(physics.Velocity2D{})
		row.PB.Set(newRigid(physics.BodyTypeStatic, physics.ColliderShape{
			ShapeType:    physics.ShapeTypeBox,
			HalfExtents:  physics.Vec2{X: 0.5, Y: 2},
			CategoryBits: 0x0001,
			MaskBits:     0xFFFF,
		}))
		wallID = id
	}, cardinal.WithHook(cardinal.Init))

	hitBefore := false
	hitAfter := false

	// At tick 10, check raycast with 0x0001 filter → should hit.
	// At tick 20, change wall to 0x0002.
	// At tick 25, check raycast with 0x0001 filter → should miss.
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		if state.Tick() == 20 {
			for eid, row := range state.Spawn.Iter() {
				if eid == wallID {
					pb := row.PB.Get()
					pb.Shapes[0].CategoryBits = 0x0002
					row.PB.Set(pb)
				}
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
	}) {
		fl := physics.Filter{CategoryBits: 0x0001, MaskBits: 0x0001}
		ray := physics.Raycast(physics.RaycastRequest{
			Origin: physics.Vec2{X: 0, Y: 0},
			End:    physics.Vec2{X: 10, Y: 0},
			Filter: &fl,
		})
		if state.Tick() == 10 {
			hitBefore = ray.Hit
		}
		if state.Tick() == 25 {
			hitAfter = ray.Hit
		}
	}, cardinal.WithHook(cardinal.PostUpdate))

	initCardinalECS(w)
	tickN(t, w, 26)

	require.True(t, hitBefore, "wall on 0x0001 should be hit by 0x0001 filter")
	require.False(t, hitAfter, "wall changed to 0x0002 should NOT be hit by 0x0001 filter")
}
