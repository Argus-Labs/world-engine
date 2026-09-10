package scenario

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/immutable"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
)

// ShapeEntities covers the contract around shapes being entities: one shape entity backs any
// number of bodies, editing it in place reaches every fixture built from it, pointing a slot
// at a different shape updates in place when only material changed and rebuilds when geometry
// changed, and chain points are the one thing fixed once a body uses them.
//
// "Rebuilt" versus "updated in place" is read off the engine: a fixture that survives keeps
// its Box2D shape id, a rebuilt one gets a new id.
func ShapeEntities() harness.Scenario {
	var s struct {
		left, right   cardinal.EntityID
		shared        physics.ShapeSlot
		leftFixture   box2d.ShapeID
		rightFixture  box2d.ShapeID
		swapSame      cardinal.EntityID
		swapSameFix   box2d.ShapeID
		swapGeom      cardinal.EntityID
		swapGeomFix   box2d.ShapeID
		editGeom      cardinal.EntityID
		editGeomSlot  physics.ShapeSlot
		editGeomFix   box2d.ShapeID
		chainSwap     cardinal.EntityID
		chainFixed    cardinal.EntityID
		chainFixedRef physics.ShapeSlot
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
	// hitY returns the Y where a downward ray at x hits id, or NaN.
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
			// Row y=0 — one shape entity on two bodies.
			s.shared = withFriction(box(1, 1), 0.3).Spawn(c)
			s.left = c.Spawn("shared-left", -5, 0, physics.NewPhysicsBody2D(physics.BodyTypeStatic, s.shared))
			s.right = c.Spawn("shared-right", 5, 0, physics.NewPhysicsBody2D(physics.BodyTypeStatic, s.shared))

			// Row y=10 — slot re-pointed at a shape with the same geometry, new material.
			s.swapSame = c.Spawn("swap-material", 0, 10, body(c, physics.BodyTypeStatic, withFriction(box(1, 1), 0.3)))

			// Row y=20 — slot re-pointed at a shape with different geometry.
			s.swapGeom = c.Spawn("swap-geometry", 0, 20, body(c, physics.BodyTypeStatic, circle(0.5)))

			// Row y=30 — the shape entity's geometry component edited in place.
			s.editGeomSlot = circle(0.5).Spawn(c)
			s.editGeom = c.Spawn("edit-geometry", 0, 30, physics.NewPhysicsBody2D(physics.BodyTypeStatic, s.editGeomSlot))

			// Row y=50 — terrain changed by pointing the slot at a new chain shape.
			s.chainSwap = c.Spawn("chain-swap", 0, 50, body(c, physics.BodyTypeStatic, chain(line(0)...)))

			// Row y=60 — a used chain shape's points edited in place, which is ignored.
			s.chainFixedRef = chain(line(0)...).Spawn(c)
			s.chainFixed = c.Spawn("chain-fixed", 0, 60,
				physics.NewPhysicsBody2D(physics.BodyTypeStatic, s.chainFixedRef))
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
				s.editGeomFix = fixture(c, s.editGeom)
				c.False("the small circle does not reach x=3 before the swap",
					c.OverlapHits(c.OverlapAABB(2.5, 19.5, 3.5, 20.5, nil), s.swapGeom), "already there")
				c.False("the small circle does not reach x=3 before the edit",
					c.OverlapHits(c.OverlapAABB(2.5, 29.5, 3.5, 30.5, nil), s.editGeom), "already there")

				if y, ok := hitY(c, s.chainSwap, 0, 50); c.True("the original chain is there", ok, "ray missed") {
					c.Near("the original polyline sits at its spawn height", y, 50, 1e-6)
				}
			}},
			{Tick: editTick, Do: func(c *harness.Ctx) {
				// In-place material edit on the shared shape entity.
				c.True("EditShape finds the shared shape", harness.EditShape(c, s.shared,
					func(common *physics.ShapeCommon, _ *physics.BoxGeom) { common.Friction = 0.9 }),
					"shape entity %d not found", s.shared.Shape)

				c.EditBody(s.swapSame, func(pb *physics.PhysicsBody2D) {
					pb.Shapes = pb.Shapes.With(0, withRestitution(withFriction(box(1, 1), 0.9), 0.5).Spawn(c))
				})
				c.EditBody(s.swapGeom, func(pb *physics.PhysicsBody2D) {
					pb.Shapes = pb.Shapes.With(0, circle(5).Spawn(c))
				})
				c.True("EditShape finds the circle", harness.EditShape(c, s.editGeomSlot,
					func(_ *physics.ShapeCommon, geom *physics.CircleGeom) { geom.Radius = 5 }),
					"shape entity %d not found", s.editGeomSlot.Shape)

				c.EditBody(s.chainSwap, func(pb *physics.PhysicsBody2D) {
					pb.Shapes = pb.Shapes.With(0, chain(line(2)...).Spawn(c))
				})
				c.True("EditShape finds the chain", harness.EditShape(c, s.chainFixedRef,
					func(_ *physics.ShapeCommon, geom *physics.ChainGeom) {
						geom.Points = immutable.SliceOf(line(2)...)
					}), "shape entity %d not found", s.chainFixedRef.Shape)
			}},
			{Tick: afterTick, Do: func(c *harness.Ctx) {
				eng := c.Plugin().Engine()

				c.True("a material edit keeps the left fixture", fixture(c, s.left) == s.leftFixture,
					"the fixture was rebuilt")
				c.True("a material edit keeps the right fixture", fixture(c, s.right) == s.rightFixture,
					"the fixture was rebuilt")
				c.Near("the edit reaches the left body's fixture", eng.ShapeFriction(s.leftFixture), 0.9, 0)
				c.Near("the edit reaches the right body's fixture", eng.ShapeFriction(s.rightFixture), 0.9, 0)

				c.True("a slot swap to the same geometry keeps the fixture",
					fixture(c, s.swapSame) == s.swapSameFix, "the fixture was rebuilt")
				c.Near("the swapped-in friction is applied", eng.ShapeFriction(s.swapSameFix), 0.9, 0)
				c.Near("the swapped-in restitution is applied", eng.ShapeRestitution(s.swapSameFix), 0.5, 0)

				c.True("a slot swap to new geometry rebuilds the fixture",
					fixture(c, s.swapGeom) != s.swapGeomFix, "the old fixture survived")
				c.True("the swapped-in radius reaches x=3",
					c.OverlapHits(c.OverlapAABB(2.5, 19.5, 3.5, 20.5, nil), s.swapGeom), "not there")

				c.True("an in-place geometry edit rebuilds the fixture",
					fixture(c, s.editGeom) != s.editGeomFix, "the old fixture survived")
				c.True("the edited radius reaches x=3",
					c.OverlapHits(c.OverlapAABB(2.5, 29.5, 3.5, 30.5, nil), s.editGeom), "not there")

				if y, ok := hitY(c, s.chainSwap, 0, 50); c.True("the swapped chain is there", ok, "ray missed") {
					c.Near("a slot swap moves the terrain", y, 52, 1e-6)
				}
				if y, ok := hitY(c, s.chainFixed, 0, 60); c.True("the fixed chain is there", ok, "ray missed") {
					c.Near("editing a used chain's points in place is ignored", y, 60, 1e-6)
				}
			}},
		},
	}
}
