package physics2d_test

// Shapes are entities: one ShapeCommon plus one geometry component, referenced from body slots
// by id. These tests pin the contract around that — sharing, in-place edits, slot swaps, and
// what happens when the referenced entity is missing.

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/stretchr/testify/require"
)

// spawnStaticAt creates a static body at pos using slot and returns its id.
func spawnStaticAt(s *spawnState, role string, pos physics.Vec2, slot physics.ShapeSlot) cardinal.EntityID {
	id, row := s.Spawn.Create()
	row.Tag.Set(harnessTag{Role: role})
	row.T.Set(physics.Transform2D{Position: pos})
	row.V.Set(physics.Velocity2D{})
	row.PB.Set(newRigid(physics.BodyTypeStatic, slot))
	return id
}

// overlapHits returns whether an AABB around pos (half size 0.5) hits entity id.
func overlapHits(p *physics.Plugin, pos physics.Vec2, id cardinal.EntityID) bool {
	ov := p.OverlapAABB(physics.AABBOverlapRequest{
		Min: physics.Vec2{X: pos.X - 0.5, Y: pos.Y - 0.5},
		Max: physics.Vec2{X: pos.X + 0.5, Y: pos.Y + 0.5},
	})
	for _, h := range ov.Hits {
		if h.Entity == id {
			return true
		}
	}
	return false
}

// firstShapeID returns the Box2D shape id backing slot 0 of entity id.
func firstShapeID(t *testing.T, p *physics.Plugin, id cardinal.EntityID) box2d.ShapeID {
	t.Helper()
	ids, ok := p.ShapeIDs(id)
	require.True(t, ok, "entity %d must have fixtures", id)
	require.NotEmpty(t, ids)
	return ids[0]
}

// TestShapeEntity_SharedAcrossBodies: one shape entity backs two bodies, and an in-place edit
// of its ShapeCommon reaches both fixtures without rebuilding them.
func TestShapeEntity_SharedAcrossBodies(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{})

	const editTick = 5
	left, right := physics.Vec2{X: -5}, physics.Vec2{X: 5}
	var leftID, rightID cardinal.EntityID
	var shared physics.ShapeSlot
	cardinal.RegisterSystem(w, func(state *spawnState) {
		switch state.Tick() {
		case 0:
			shared = spawnShape(state, physics.Box(1, 1).Material(0.3, 0, 0).Filter(0xFFFF, 0xFFFF))
			leftID = spawnStaticAt(state, "left", left, shared)
			rightID = spawnStaticAt(state, "right", right, shared)
		case editTick:
			shape, err := state.Boxes.GetByID(shared.Shape)
			require.NoError(t, err)
			c := shape.Common.Get()
			c.Friction = 0.9
			shape.Common.Set(c)
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, editTick)
	require.True(t, overlapHits(p, left, leftID), "left body carries the shared box")
	require.True(t, overlapHits(p, right, rightID), "right body carries the shared box")
	leftBefore, rightBefore := firstShapeID(t, p, leftID), firstShapeID(t, p, rightID)
	require.InDelta(t, 0.3, p.Engine().ShapeFriction(leftBefore), 0)

	tickN(t, w, 2) // edit tick, then one pipeline tick applies it
	require.Equal(t, leftBefore, firstShapeID(t, p, leftID), "material edit must not rebuild fixtures")
	require.Equal(t, rightBefore, firstShapeID(t, p, rightID), "material edit must not rebuild fixtures")
	require.InDelta(t, 0.9, p.Engine().ShapeFriction(leftBefore), 0, "left fixture sees the new friction")
	require.InDelta(t, 0.9, p.Engine().ShapeFriction(rightBefore), 0, "right fixture sees the new friction")
}

// TestShapeEntity_SlotSwapSameGeometryInPlace: pointing a slot at a different shape entity with
// the same geometry only touches material and filter — the fixture survives.
func TestShapeEntity_SlotSwapSameGeometryInPlace(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{})

	const swapTick = 5
	var bodyID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *spawnState) {
		switch state.Tick() {
		case 0:
			bodyID = spawnStaticAt(state, "swap", physics.Vec2{},
				spawnShape(state, physics.Box(1, 1).Material(0.3, 0, 0).Filter(0xFFFF, 0xFFFF)))
		case swapTick:
			for eid, row := range state.Spawn.Iter() {
				if eid == bodyID {
					pb := row.PB.Get()
					pb.Shapes = pb.Shapes.With(0, spawnShape(state, physics.Box(1, 1).Material(0.9, 0.5, 0).Filter(0xFFFF, 0xFFFF)))
					row.PB.Set(pb)
				}
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, swapTick)
	before := firstShapeID(t, p, bodyID)

	tickN(t, w, 2)
	require.Equal(t, before, firstShapeID(t, p, bodyID), "same geometry: fixture kept")
	require.InDelta(t, 0.9, p.Engine().ShapeFriction(before), 0)
	require.InDelta(t, 0.5, p.Engine().ShapeRestitution(before), 0)
}

// TestShapeEntity_SlotSwapNewGeometryRebuilds: a slot swap to a shape with different geometry
// recreates the fixture.
func TestShapeEntity_SlotSwapNewGeometryRebuilds(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{})

	const swapTick = 5
	var bodyID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *spawnState) {
		switch state.Tick() {
		case 0:
			bodyID = spawnStaticAt(state, "grow", physics.Vec2{},
				spawnShape(state, physics.Circle(0.5).Material(0, 0, 0).Filter(0xFFFF, 0xFFFF)))
		case swapTick:
			for eid, row := range state.Spawn.Iter() {
				if eid == bodyID {
					pb := row.PB.Get()
					pb.Shapes = pb.Shapes.With(0, spawnShape(state, physics.Circle(5).Material(0, 0, 0).Filter(0xFFFF, 0xFFFF)))
					row.PB.Set(pb)
				}
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, swapTick)
	before := firstShapeID(t, p, bodyID)
	require.False(t, overlapHits(p, physics.Vec2{X: 3}, bodyID), "radius 0.5 does not reach X=3")

	tickN(t, w, 2)
	require.NotEqual(t, before, firstShapeID(t, p, bodyID), "new geometry: fixture rebuilt")
	require.True(t, overlapHits(p, physics.Vec2{X: 3}, bodyID), "radius 5 reaches X=3")
}

// TestShapeEntity_MissingFailsLoud: a slot whose shape entity does not exist fails the body's
// creation (logged, no crash) and leaves no fixture behind.
func TestShapeEntity_MissingFailsLoud(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{})

	var bodyID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *spawnState) {
		if state.Tick() == 0 {
			bodyID = spawnStaticAt(state, "dangling", physics.Vec2{}, physics.Slot(999_999)) // no such entity
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, 3)
	_, ok := p.ShapeIDs(bodyID)
	require.False(t, ok, "no fixture may exist for a body whose shape entity is missing")
}

// TestShapeEntity_DeletedShapeFailsOnRebuild: the plugin never deletes shape entities and a
// body keeps its fixtures if the game deletes one out from under it, but the next full rebuild
// (Reset, restore) cannot resolve the slot and fails that body loudly.
func TestShapeEntity_DeletedShapeFailsOnRebuild(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{})

	const deleteTick = 5
	var bodyID cardinal.EntityID
	var slot physics.ShapeSlot
	cardinal.RegisterSystem(w, func(state *spawnState) {
		switch state.Tick() {
		case 0:
			slot = spawnShape(state, physics.Box(1, 1).Material(0, 0, 0).Filter(0xFFFF, 0xFFFF))
			bodyID = spawnStaticAt(state, "orphaned", physics.Vec2{}, slot)
		case deleteTick:
			require.True(t, state.Boxes.Destroy(slot.Shape))
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, deleteTick+2)
	require.True(t, overlapHits(p, physics.Vec2{}, bodyID), "fixtures stay until the body is rebuilt")

	p.Reset()
	tickN(t, w, 2)
	_, ok := p.ShapeIDs(bodyID)
	require.False(t, ok, "rebuild cannot resolve the deleted shape entity")
}

// TestShapeEntity_TwoGeometriesFirstKindWins: a shape entity carrying two geometry components
// is a game bug; the plugin logs it and uses the first kind it gathers (search order:
// circle, box, polygon, chain, edge, capsule) so the world keeps running deterministically.
func TestShapeEntity_TwoGeometriesFirstKindWins(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{})

	type twoGeomRow struct {
		Common cardinal.Ref[physics.ShapeCommon]
		Circle cardinal.Ref[physics.CircleGeom]
		Box    cardinal.Ref[physics.BoxGeom]
	}
	var bodyID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *struct {
		cardinal.BaseSystemState
		Spawn spawnArchetype
		Both  cardinal.Exact[twoGeomRow]
	}) {
		if state.Tick() != 0 {
			return
		}
		id, row := state.Both.Create()
		c := physics.Circle(0.5).Common
		c.CategoryBits, c.MaskBits = 0xFFFF, 0xFFFF
		row.Common.Set(c)
		row.Circle.Set(physics.CircleGeom{Radius: 0.5})
		row.Box.Set(physics.BoxGeom{HalfExtents: physics.Vec2{X: 5, Y: 5}})

		bid, body := state.Spawn.Create()
		body.Tag.Set(harnessTag{Role: "ambiguous"})
		body.T.Set(physics.Transform2D{})
		body.V.Set(physics.Velocity2D{})
		body.PB.Set(newRigid(physics.BodyTypeStatic, physics.Slot(id)))
		bodyID = bid
	}, cardinal.WithHook(cardinal.Init))

	initCardinalECS(w)
	tickN(t, w, 3)
	require.True(t, overlapHits(p, physics.Vec2{}, bodyID), "the circle is attached")
	require.False(t, overlapHits(p, physics.Vec2{X: 4}, bodyID), "the box is not")
}

// TestShapeEntity_InPlaceGeometryEditRebuilds: editing a shape entity's geometry component in
// place is a structural change; the fixtures of every body using it are rebuilt.
func TestShapeEntity_InPlaceGeometryEditRebuilds(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{})

	const editTick = 5
	var bodyID cardinal.EntityID
	var slot physics.ShapeSlot
	cardinal.RegisterSystem(w, func(state *spawnState) {
		switch state.Tick() {
		case 0:
			slot = spawnShape(state, physics.Circle(0.5).Material(0, 0, 0).Filter(0xFFFF, 0xFFFF))
			bodyID = spawnStaticAt(state, "resized", physics.Vec2{}, slot)
		case editTick:
			shape, err := state.Circles.GetByID(slot.Shape)
			require.NoError(t, err)
			shape.Geom.Set(physics.CircleGeom{Radius: 5})
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, editTick)
	before := firstShapeID(t, p, bodyID)
	require.False(t, overlapHits(p, physics.Vec2{X: 3}, bodyID), "radius 0.5 does not reach X=3")

	tickN(t, w, 2)
	require.NotEqual(t, before, firstShapeID(t, p, bodyID), "geometry edit: fixture rebuilt")
	require.True(t, overlapHits(p, physics.Vec2{X: 3}, bodyID), "radius 5 reaches X=3")
}

// ---------------------------------------------------------------------------
// Chain points are fixed per shape entity: swap the slot to change terrain
// ---------------------------------------------------------------------------

// TestChainShape_SwapRebuildsFixture covers the chain contract: points are fixed once a shape
// is used, so pointing the slot at a new chain shape is the way to change terrain, and the
// reconciler must rebuild the fixture from that slot change.
func TestChainShape_SwapRebuildsFixture(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	// Right-to-left winding: upward-facing normals, so a downward ray hits (see chain tests above).
	lineAt := func(y float64) []physics.Vec2 {
		return []physics.Vec2{{X: 10, Y: y}, {X: 3, Y: y}, {X: -3, Y: y}, {X: -10, Y: y}}
	}

	const swapTick = 5
	var chainID cardinal.EntityID
	cardinal.RegisterSystem(w, func(state *spawnState) {
		switch state.Tick() {
		case 0:
			id, row := state.Spawn.Create()
			row.Tag.Set(harnessTag{Role: "swap_chain"})
			row.T.Set(physics.Transform2D{})
			row.V.Set(physics.Velocity2D{})
			row.PB.Set(newRigid(physics.BodyTypeStatic, spawnShape(state, physics.Chain(lineAt(0)...))))
			chainID = id
		case swapTick:
			for eid, row := range state.Spawn.Iter() {
				if eid != chainID {
					continue
				}
				pb := row.PB.Get()
				pb.Shapes = pb.Shapes.With(0, spawnShape(state, physics.Chain(lineAt(2)...)))
				row.PB.Set(pb)
			}
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, swapTick)

	ray := p.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 5},
		End:    physics.Vec2{X: 0, Y: -5},
	})
	require.True(t, ray.Hit, "raycast should hit the original chain")
	require.Equal(t, chainID, ray.Entity)
	require.InDelta(t, 0.0, ray.Point.Y, 1e-9, "original polyline sits at y=0")

	tickN(t, w, 2) // swap tick runs, then one pipeline tick reconciles the slot change

	ray = p.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 5},
		End:    physics.Vec2{X: 0, Y: -5},
	})
	require.True(t, ray.Hit, "raycast should hit the swapped chain")
	require.Equal(t, chainID, ray.Entity)
	require.InDelta(t, 2.0, ray.Point.Y, 1e-9, "swapped polyline sits at y=2")
}

// TestChainShape_PointsFixedOnceUsed pins the other half of the contract: editing a used chain
// shape's points in place is ignored (the plugin keeps its first copy), so the fixture stays.
func TestChainShape_PointsFixedOnceUsed(t *testing.T) {
	t.Parallel()
	w, p := makeWorld(t, physics.Vec2{X: 0, Y: 0})

	lineAt := func(y float64) []physics.Vec2 {
		return []physics.Vec2{{X: 10, Y: y}, {X: 3, Y: y}, {X: -3, Y: y}, {X: -10, Y: y}}
	}

	const editTick = 5
	var slot physics.ShapeSlot
	cardinal.RegisterSystem(w, func(state *spawnState) {
		switch state.Tick() {
		case 0:
			slot = spawnShape(state, physics.Chain(lineAt(0)...))
			_, row := state.Spawn.Create()
			row.Tag.Set(harnessTag{Role: "fixed_chain"})
			row.T.Set(physics.Transform2D{})
			row.V.Set(physics.Velocity2D{})
			row.PB.Set(newRigid(physics.BodyTypeStatic, slot))
		case editTick:
			shape, err := state.Chains.GetByID(slot.Shape)
			require.NoError(t, err)
			shape.Geom.Set(physics.ChainGeom{Points: immutable.SliceOf(lineAt(2)...)})
		}
	}, cardinal.WithHook(cardinal.Update))

	initCardinalECS(w)
	tickN(t, w, editTick+2)

	ray := p.Raycast(physics.RaycastRequest{
		Origin: physics.Vec2{X: 0, Y: 5},
		End:    physics.Vec2{X: 0, Y: -5},
	})
	require.True(t, ray.Hit)
	require.InDelta(t, 0.0, ray.Point.Y, 1e-9, "in-place point edits are ignored; the y=0 polyline stays")
}
