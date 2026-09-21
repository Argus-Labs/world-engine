package scenario

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
)

// ShapeEntities covers the contract around shapes being entities: one shape entity backs any
// number of bodies, a shape is never changed in place, and Fork is how a body gets a changed
// shape without touching the bodies that still share the original. Pointing a slot at a
// shape with the same geometry updates the fixture in place; different geometry rebuilds it.
//
// "Rebuilt" versus "updated in place" is read off the engine: a fixture that survives keeps
// its Box2D shape id, a rebuilt one gets a new id.
func ShapeEntities() harness.Scenario {
	var s struct {
		left, right  cardinal.EntityID
		shared       physics.ShapeRef
		leftFixture  box2d.ShapeID
		rightFixture box2d.ShapeID
		swapSame     cardinal.EntityID
		swapSameFix  box2d.ShapeID
		swapGeom     cardinal.EntityID
		swapGeomFix  box2d.ShapeID
		forkGeom     cardinal.EntityID
		forkGeomSlot physics.ShapeRef
		forkGeomFix  box2d.ShapeID
		chainSwap    cardinal.EntityID
	}

	const (
		beforeTick = 3
		editTick   = 5
		afterTick  = 8
	)

	// fixture returns the Box2D shape id behind slot 0 of id, or a zero id when the body has
	// no fixtures.
	fixture := func(c *harness.Ctx, id cardinal.EntityID) box2d.ShapeID {
		ids, ok := c.Plugin().ShapeIDs(id)
		if !ok || len(ids) == 0 {
			return box2d.ShapeID{}
		}
		return ids[0]
	}
	// line is a right-to-left chain at height y, so a downward ray hits its upward normal.
	line := func(y float64) []physics.Vec2 {
		return []physics.Vec2{vec(10, y), vec(3, y), vec(-3, y), vec(-10, y)}
	}
	// hitY returns the Y where a downward ray at x hits id.
	hitY := func(c *harness.Ctx, id cardinal.EntityID, x, top float64) (float64, bool) {
		res := c.Raycast(x, top+5, x, top-5, nil)
		if !res.Hit || res.Entity != id {
			return 0, false
		}
		return res.Point.Y, true
	}

	return harness.Scenario{
		Name: "shape-entities",
		Setup: func(c *harness.Ctx) {
			// Row y=0 — one shape entity on two bodies. The left one is later re-pointed at
			// a Fork; the right one must not notice.
			s.shared = withFriction(box(1, 1), 0.3).Spawn(c)
			s.left = c.Spawn("shared-left", -5, 0, physics.NewPhysicsBody2D(physics.BodyTypeStatic, s.shared))
			s.right = c.Spawn("shared-right", 5, 0, physics.NewPhysicsBody2D(physics.BodyTypeStatic, s.shared))

			// Spawn de-duplicates: an equal definition is the same shape entity, a different
			// one is not.
			again := withFriction(box(1, 1), 0.3).Spawn(c)
			c.True("Spawn returns the existing shape for an equal definition",
				again.Shape == s.shared.Shape, "got %d, want %d", again.Shape, s.shared.Shape)
			other := withFriction(box(1, 1), 0.6).Spawn(c)
			c.True("Spawn creates a new shape for a different material",
				other.Shape != s.shared.Shape, "shared the entity")

			// Row y=10 — slot re-pointed at a shape with the same geometry, new material.
			s.swapSame = c.Spawn("swap-material", 0, 10, body(c, physics.BodyTypeStatic, withFriction(box(1, 1), 0.3)))

			// Row y=20 — slot re-pointed at a shape with different geometry.
			s.swapGeom = c.Spawn("swap-geometry", 0, 20, body(c, physics.BodyTypeStatic, circle(0.5)))

			// Row y=30 — a Fork that changes geometry.
			s.forkGeomSlot = circle(0.5).Spawn(c)
			s.forkGeom = c.Spawn("fork-geometry", 0, 30, physics.NewPhysicsBody2D(physics.BodyTypeStatic, s.forkGeomSlot))

			// Row y=50 — terrain changed by pointing the slot at a new chain shape.
			s.chainSwap = c.Spawn("chain-swap", 0, 50, body(c, physics.BodyTypeStatic, chain(line(0)...)))
		},
		Steps: []harness.Step{
			{Tick: beforeTick, Do: func(c *harness.Ctx) {
				c.True("both bodies carry the shared shape",
					c.OverlapHits(c.OverlapAABB(-5.5, -0.5, -4.5, 0.5, nil), s.left) &&
						c.OverlapHits(c.OverlapAABB(4.5, -0.5, 5.5, 0.5, nil), s.right),
					"a body built from the shared shape has no fixture")
				s.leftFixture, s.rightFixture = fixture(c, s.left), fixture(c, s.right)
				c.Near("the shared shape's friction reached the engine",
					c.Plugin().Engine().ShapeFriction(s.leftFixture), 0.3, 0)
				s.swapSameFix = fixture(c, s.swapSame)
				s.swapGeomFix = fixture(c, s.swapGeom)
				s.forkGeomFix = fixture(c, s.forkGeom)
				c.False("the small circle does not reach x=3 before the swap",
					c.OverlapHits(c.OverlapAABB(2.5, 19.5, 3.5, 20.5, nil), s.swapGeom), "already there")
				c.False("the small circle does not reach x=3 before the fork",
					c.OverlapHits(c.OverlapAABB(2.5, 29.5, 3.5, 30.5, nil), s.forkGeom), "already there")

				if def, ok := harness.ReadShape(c, s.shared); c.True("Read finds the shared box", ok, "") {
					c.True("Read reports the kind", def.Kind() == physics.KindBox, "got %s", def.Kind())
					c.Near("Read returns the shape's friction", def.Friction(), 0.3, 0)
					c.NearVec("Read returns the shape's geometry", def.HalfExtents(), vec(1, 1), 0)
				}

				if y, ok := hitY(c, s.chainSwap, 0, 50); c.True("the original chain is there", ok, "ray missed") {
					c.Near("the original polyline sits at its spawn height", y, 50, 1e-6)
				}
			}},
			{Tick: editTick, Do: func(c *harness.Ctx) {
				// Fork the shared shape with new friction and re-point the left body only.
				mine, err := harness.ForkShape(c, s.shared, func(d physics.Shape) physics.Shape {
					return d.Material(0.9, d.Restitution(), d.Density())
				})
				if c.NoError("Fork copies the shared box", err) {
					c.True("Fork returns a new shape entity", mine.Shape != s.shared.Shape, "same id as the original")
					c.EditBody(s.left, func(pb *physics.PhysicsBody2D) { pb.Shapes = pb.Shapes.With(0, mine) })
				}

				c.EditBody(s.swapSame, func(pb *physics.PhysicsBody2D) {
					pb.Shapes = pb.Shapes.With(0, withRestitution(withFriction(box(1, 1), 0.9), 0.5).Spawn(c))
				})
				c.EditBody(s.swapGeom, func(pb *physics.PhysicsBody2D) {
					pb.Shapes = pb.Shapes.With(0, circle(5).Spawn(c))
				})

				bigger, err := harness.ForkShape(c, s.forkGeomSlot, func(d physics.Shape) physics.Shape {
					return d.Reshape(physics.Circle(5))
				})
				if c.NoError("Fork copies the circle", err) {
					c.EditBody(s.forkGeom, func(pb *physics.PhysicsBody2D) { pb.Shapes = pb.Shapes.With(0, bigger) })
				}

				c.EditBody(s.chainSwap, func(pb *physics.PhysicsBody2D) {
					pb.Shapes = pb.Shapes.With(0, chain(line(2)...).Spawn(c))
				})
			}},
			{Tick: afterTick, Do: func(c *harness.Ctx) {
				eng := c.Plugin().Engine()

				c.True("re-pointing at a Fork with the same geometry keeps the fixture",
					fixture(c, s.left) == s.leftFixture, "the fixture was rebuilt")
				c.Near("the Fork's friction reaches the body that took it", eng.ShapeFriction(s.leftFixture), 0.9, 0)
				c.True("the body still on the original keeps its fixture",
					fixture(c, s.right) == s.rightFixture, "the fixture was rebuilt")
				c.Near("the original shape is untouched by the Fork", eng.ShapeFriction(s.rightFixture), 0.3, 0)
				if def, ok := harness.ReadShape(c, s.shared); c.True("the original still reads", ok, "") {
					c.Near("Read confirms the original's friction did not change", def.Friction(), 0.3, 0)
				}

				c.True("a slot swap to the same geometry keeps the fixture",
					fixture(c, s.swapSame) == s.swapSameFix, "the fixture was rebuilt")
				c.Near("the swapped-in friction is applied", eng.ShapeFriction(s.swapSameFix), 0.9, 0)
				c.Near("the swapped-in restitution is applied", eng.ShapeRestitution(s.swapSameFix), 0.5, 0)

				c.True("a slot swap to new geometry rebuilds the fixture",
					fixture(c, s.swapGeom) != s.swapGeomFix, "the old fixture survived")
				c.True("the swapped-in radius reaches x=3",
					c.OverlapHits(c.OverlapAABB(2.5, 19.5, 3.5, 20.5, nil), s.swapGeom), "not there")

				c.True("a Fork with new geometry rebuilds the fixture",
					fixture(c, s.forkGeom) != s.forkGeomFix, "the old fixture survived")
				c.True("the forked radius reaches x=3",
					c.OverlapHits(c.OverlapAABB(2.5, 29.5, 3.5, 30.5, nil), s.forkGeom), "not there")

				if y, ok := hitY(c, s.chainSwap, 0, 50); c.True("the swapped chain is there", ok, "ray missed") {
					c.Near("a slot swap moves the terrain", y, 52, 1e-6)
				}
			}},
		},
	}
}
