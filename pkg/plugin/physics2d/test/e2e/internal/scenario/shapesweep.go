package scenario

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
)

// ShapeSweep covers automatic shape cleanup: a shape entity lives exactly as long as some body
// names it. One spawned without a body is gone after the next reconcile, a body's death or a
// slot swap releases its shape, a shape traded between two bodies in one tick survives, and a
// Reset starts the counts over under the same rule.
//
// The Reset happens on the same tick the reset scenario uses, so a run that puts every
// scenario in one world sees one rebuild, not two.
// sweepBox spawns a box with its own friction. Spawn de-duplicates equal definitions, and
// this scenario watches individual shapes live and die, so each one must be distinct.
func sweepBox(c *harness.Ctx, friction float64) physics.ShapeRef {
	return withFriction(box(1, 1), friction).Spawn(c)
}

func ShapeSweep() harness.Scenario {
	var s struct {
		lone, loneShape        cardinal.EntityID
		swapper                cardinal.EntityID
		swapOld, swapNew       physics.ShapeRef
		first, second, shared  cardinal.EntityID
		staged                 cardinal.EntityID
		a, b                   cardinal.EntityID
		shapeA, shapeB         physics.ShapeRef
		survivor, survivorSlot cardinal.EntityID
		broken                 cardinal.EntityID
		brokenKeep, brokenGone physics.ShapeRef
	}

	const (
		brokenShapeGone = 5
		brokenBodyGone  = 9
		destroyTick     = 5
		swapTick        = 5
		firstGone       = 3
		secondGone      = 6
		resetTick       = 450 // must match scenario.Reset
	)

	wall := func(c *harness.Ctx, label string, x, y float64, slot physics.ShapeRef) cardinal.EntityID {
		return c.Spawn(label, x, y, physics.NewPhysicsBody2D(physics.BodyTypeStatic, slot))
	}

	return harness.Scenario{
		Name: "shape-sweep",
		Setup: func(c *harness.Ctx) {
			// Row y=0 — one body, one shape; the body dies.
			slot := sweepBox(c, 0.301)
			s.loneShape = slot.Shape
			s.lone = wall(c, "lone-wall", 0, 0, slot)

			// Row y=10 — a slot swap releases the old shape.
			s.swapOld = sweepBox(c, 0.302)
			s.swapper = wall(c, "swapper", 0, 10, s.swapOld)

			// Row y=20 — two bodies share a shape; they die one at a time.
			shared := sweepBox(c, 0.303)
			s.shared = shared.Shape
			s.first = wall(c, "shared-first", -5, 20, shared)
			s.second = wall(c, "shared-second", 5, 20, shared)

			// A shape no body ever uses: swept by the first reconcile.
			s.staged = sweepBox(c, 0.304).Shape

			// Row y=30 — two bodies trade shapes in one tick.
			s.shapeA, s.shapeB = sweepBox(c, 0.305), sweepBox(c, 0.306)
			s.a = wall(c, "trade-a", -5, 30, s.shapeA)
			s.b = wall(c, "trade-b", 5, 30, s.shapeB)

			// Row y=50 — a body that stops attaching, then leaves. It names two shapes; one is
			// deleted, so its rebuild fails every tick and it holds no shadow. The shapes it
			// still names must live until the entity itself goes, and go with it.
			s.brokenKeep = sweepBox(c, 0.307)
			s.brokenGone = sweepBox(c, 0.308).At(vec(5, 0), 0)
			s.broken = c.Spawn("broken-then-gone", 0, 50,
				physics.NewPhysicsBody2D(physics.BodyTypeStatic, s.brokenKeep, s.brokenGone))

			// Row y=40 — its body dies in the tick the world is reset.
			slot = sweepBox(c, 0.309)
			s.survivorSlot = slot.Shape
			s.survivor = wall(c, "reset-orphan", 0, 40, slot)
		},
		Steps: []harness.Step{
			{Tick: 2, Do: func(c *harness.Ctx) {
				for _, id := range []cardinal.EntityID{s.loneShape, s.swapOld.Shape, s.shared,
					s.shapeA.Shape, s.shapeB.Shape, s.survivorSlot} {
					c.True("every used shape exists before anything is released", c.ShapeAlive(id),
						"shape entity %d is missing", id)
				}
				c.False("a shape spawned with no body is swept by the next reconcile", c.ShapeAlive(s.staged),
					"shape entity %d still exists", s.staged)
			}},
			{Tick: brokenShapeGone, Do: func(c *harness.Ctx) {
				c.True("deleting one of the broken body's shapes succeeds",
					c.DestroyShape(s.brokenGone.Shape), "Destroy returned false")
			}},
			{Tick: brokenShapeGone + 2, Do: func(c *harness.Ctx) {
				c.True("a shape a failing body still names is kept", c.ShapeAlive(s.brokenKeep.Shape),
					"shape entity %d was swept while its body was failing", s.brokenKeep.Shape)
			}},
			{Tick: brokenBodyGone, Do: func(c *harness.Ctx) {
				c.True("destroying the failing body succeeds", c.Destroy(s.broken), "Destroy returned false")
			}},
			{Tick: brokenBodyGone + 2, Do: func(c *harness.Ctx) {
				c.False("its shapes are released once the entity itself is gone",
					c.ShapeAlive(s.brokenKeep.Shape),
					"shape entity %d outlived the only body that named it", s.brokenKeep.Shape)
			}},
			{Tick: firstGone, Do: func(c *harness.Ctx) {
				c.True("destroying the first sharer succeeds", c.Destroy(s.first), "Destroy returned false")
			}},
			{Tick: destroyTick, Do: func(c *harness.Ctx) {
				c.True("destroying the lone body succeeds", c.Destroy(s.lone), "Destroy returned false")
				s.swapNew = sweepBox(c, 0.310)
				c.EditBody(s.swapper, func(pb *physics.PhysicsBody2D) { pb.Shapes = pb.Shapes.With(0, s.swapNew) })
				c.EditBody(s.a, func(pb *physics.PhysicsBody2D) { pb.Shapes = pb.Shapes.With(0, s.shapeB) })
				c.EditBody(s.b, func(pb *physics.PhysicsBody2D) { pb.Shapes = pb.Shapes.With(0, s.shapeA) })
			}},
			{Tick: secondGone, Do: func(c *harness.Ctx) {
				c.True("a shared shape survives while one body still uses it", c.ShapeAlive(s.shared),
					"the shape went with the first body")
				c.True("destroying the second sharer succeeds", c.Destroy(s.second), "Destroy returned false")
			}},
			{Tick: 8, Do: func(c *harness.Ctx) {
				c.False("a shape is swept once its last body is gone", c.ShapeAlive(s.loneShape),
					"shape entity %d still exists", s.loneShape)
				c.False("a slot swap sweeps the old shape", c.ShapeAlive(s.swapOld.Shape),
					"shape entity %d still exists", s.swapOld.Shape)
				c.True("a slot swap keeps the new shape", c.ShapeAlive(s.swapNew.Shape),
					"shape entity %d was swept", s.swapNew.Shape)
				c.False("a shared shape is swept when its last body is gone", c.ShapeAlive(s.shared),
					"shape entity %d still exists", s.shared)
				c.True("shapes traded between bodies in one tick both survive",
					c.ShapeAlive(s.shapeA.Shape) && c.ShapeAlive(s.shapeB.Shape), "a traded shape was swept")
				c.True("the traded bodies still have fixtures",
					c.OverlapHits(c.OverlapAABB(-5.5, 29.5, -4.5, 30.5, nil), s.a) &&
						c.OverlapHits(c.OverlapAABB(4.5, 29.5, 5.5, 30.5, nil), s.b), "a traded body lost its fixture")
			}},
			{Tick: resetTick, Do: func(c *harness.Ctx) {
				// Destroy the body and drop the runtime in one tick: the rebuild counts from
				// the bodies that remain, and this shape has none.
				c.True("destroying the reset orphan's body succeeds", c.Destroy(s.survivor),
					"Destroy returned false")
				c.ExpectWorldReset()
				c.Plugin().Reset()
			}},
			{Tick: resetTick + 10, Do: func(c *harness.Ctx) {
				c.False("after a Reset a shape no body names is swept like any other",
					c.ShapeAlive(s.survivorSlot), "shape entity %d survived the rebuild", s.survivorSlot)
			}},
		},
	}
}
