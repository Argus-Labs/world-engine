package scenario

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
)

// ShapeStore covers kept shapes: a shape in the store outlives having no body, can be found
// again from its definition, can be kept mid-game, and once released goes back to the normal
// rule of living exactly as long as some body names it.
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
			c.Store().Keep(s.bullet)
			c.Store().Keep(s.bullet) // twice is fine
			c.HasError("Release refuses a shape that is not kept",
				c.Store().Release(withFriction(circle(0.2), 0.37).Spawn(c)))
		},
		Steps: []harness.Step{
			{Tick: 3, Do: func(c *harness.Ctx) {
				c.True("a kept shape survives with no body", c.ShapeAlive(s.bullet.Shape),
					"shape entity %d was swept", s.bullet.Shape)
				ref, ok := c.Shapes().Find(physics.Circle(0.1).Material(0.37, defaultRestitution, defaultDensity))
				c.True("Find gets a kept shape back from its definition", ok && ref.Shape == s.bullet.Shape, "got %+v", ref)
				_, ok = c.Shapes().Find(physics.Circle(0.11).Material(0.37, defaultRestitution, defaultDensity))
				c.False("Find reports a definition no live shape matches", ok, "found something")
				s.body = c.Spawn("bullet-body", 0, 0, physics.NewPhysicsBody2D(physics.BodyTypeStatic, ref))
			}},
			{Tick: 6, Do: func(c *harness.Ctx) {
				c.True("a body built from a kept shape has its fixture",
					c.OverlapHits(c.OverlapAABB(-0.2, -0.2, 0.2, 0.2, nil), s.body), "no hit")
				// A power-up: keep a new shape mid-game, with no body yet.
				s.dash = withFriction(capsule(vec(0, 0), vec(1, 0), 0.25), 0.37).Spawn(c)
				c.Store().Keep(s.dash)
			}},
			{Tick: 9, Do: func(c *harness.Ctx) {
				c.True("a shape kept mid-game survives with no body", c.ShapeAlive(s.dash.Shape),
					"shape entity %d was swept", s.dash.Shape)
				c.NoError("Release drops a kept shape", c.Store().Release(s.dash))
				c.HasError("Release refuses a released shape", c.Store().Release(s.dash))
			}},
			{Tick: 10, Do: func(c *harness.Ctx) {
				c.False("a released shape with no body is swept", c.ShapeAlive(s.dash.Shape),
					"shape entity %d survived", s.dash.Shape)
				c.NoError("Release a shape a body still uses", c.Store().Release(s.bullet))
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
