package scenario

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
)

// ShapeStore covers kept shapes: a shape in the store outlives having no body, can be
// looked up by name from any tick, can be kept mid-game, and once released goes back to the
// normal rule of living exactly as long as some body names it.
//
// Sweep checks run on the tick right after the sweep: Cardinal recycles freed entity ids, so
// a later check could find another scenario's new shape under the same id.
func ShapeStore() harness.Scenario {
	var s struct {
		bullet physics.ShapeRef
		dash   physics.ShapeRef
		body   cardinal.EntityID
	}

	return harness.Scenario{
		Name: "shape-store",
		Setup: func(c *harness.Ctx) {
			// Distinct materials: Spawn de-duplicates, and this scenario watches these
			// shapes live and die, so they must not be shared with another scenario.
			s.bullet = withFriction(circle(0.1), 0.37).Spawn(c)
			c.NoError("Keep at init", c.Store().Keep("bullet", s.bullet))
			c.HasError("Keep refuses a name already in use", c.Store().Keep("bullet", withFriction(circle(0.2), 0.37).Spawn(c)))
			c.HasError("Keep refuses an empty name", c.Store().Keep("", s.bullet))
		},
		Steps: []harness.Step{
			{Tick: 3, Do: func(c *harness.Ctx) {
				c.True("a kept shape survives with no body", c.ShapeAlive(s.bullet.Shape),
					"shape entity %d was swept", s.bullet.Shape)
				ref, ok := c.Store().Get("bullet")
				c.True("Kept finds a shape by name", ok && ref.Shape == s.bullet.Shape, "got %+v", ref)
				_, ok = c.Store().Get("nope")
				c.False("Kept reports an unknown name", ok, "found something")
				s.body = c.Spawn("bullet-body", 0, 0, physics.NewPhysicsBody2D(physics.BodyTypeStatic, ref))
			}},
			{Tick: 6, Do: func(c *harness.Ctx) {
				c.True("a body built from a kept shape has its fixture",
					c.OverlapHits(c.OverlapAABB(-0.2, -0.2, 0.2, 0.2, nil), s.body), "no hit")
				// A power-up: keep a new shape mid-game, with no body yet.
				s.dash = withFriction(capsule(vec(0, 0), vec(1, 0), 0.25), 0.37).Spawn(c)
				c.NoError("Keep mid-game", c.Store().Keep("dash", s.dash))
			}},
			{Tick: 9, Do: func(c *harness.Ctx) {
				c.True("a shape kept mid-game survives with no body", c.ShapeAlive(s.dash.Shape),
					"shape entity %d was swept", s.dash.Shape)
				c.NoError("Release drops a kept shape", c.Store().Release("dash"))
				c.HasError("Release refuses an unknown name", c.Store().Release("nope"))
			}},
			{Tick: 10, Do: func(c *harness.Ctx) {
				c.False("a released shape with no body is swept", c.ShapeAlive(s.dash.Shape),
					"shape entity %d survived", s.dash.Shape)
				_, ok := c.Store().Get("dash")
				c.False("a released name is gone from the store", ok, "still kept")
				c.NoError("Release a shape a body still uses", c.Store().Release("bullet"))
			}},
			{Tick: 15, Do: func(c *harness.Ctx) {
				c.True("a released shape stays while a body names it", c.ShapeAlive(s.bullet.Shape),
					"shape entity %d was swept", s.bullet.Shape)
				c.Destroy(s.body)
			}},
			{Tick: 16, Do: func(c *harness.Ctx) {
				c.False("a released shape is swept once its last body is gone", c.ShapeAlive(s.bullet.Shape),
					"shape entity %d survived", s.bullet.Shape)
			}},
		},
	}
}
