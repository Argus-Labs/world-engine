package scenario

import (
	"sort"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	physcomp "github.com/argus-labs/world-engine/pkg/plugin/physics2d/internal/component"
)

// Robustness holds the inputs a game can hand the plugin that the plugin has to survive: a
// shape Box2D cannot build, and an entity destroyed while it still holds a live contact.
//
// The bad shapes split in two, and only the first half is what this file was written for.
// A chain on a dynamic body is finite and well formed, so it passes component validation and
// reaches the engine; against the cgo bridge inputs like this tripped a fatal Box2D assertion
// and killed the shard, and the pure-Go engine must not die. The rest — a radius of zero or
// less, a box with no extent, a polygon outside 3..MaxPolygonVertices, a chain under four
// points or marked as a sensor, an edge or capsule whose endpoints meet — are refused by
// Validate and never reach Box2D. They stay here as rejected cases so the refusal is
// measured too.
//
// Each still runs alone via -hostile <name>, because a case that does regress to
// killing the process would otherwise hide every case after it. Every case
// prints what it is about to do first, so the log says which one died even when
// the process never reaches the summary.
func Hostile(name string) (harness.Scenario, bool) {
	for _, sc := range hostileCases() {
		if sc.Name == name {
			return sc, true
		}
	}
	return harness.Scenario{}, false
}

// HostileNames lists the available -hostile cases, in a stable order.
func HostileNames() []string {
	var names []string
	for _, sc := range hostileCases() {
		names = append(names, sc.Name)
	}
	sort.Strings(names)
	return names
}

func hostileCases() []harness.Scenario {
	return []harness.Scenario{
		hostileDestroyDuringContact(),
		hostileRejectedShape("short-chain",
			"a chain of 3 points (Box2D needs 4)",
			chain(vec(-3, 0), vec(0, 0), vec(3, 0))),
		hostileRejectedShape("short-chain-loop",
			"a chain loop of 3 points (Box2D needs 4)",
			chainLoop(vec(-3, 0), vec(0, 3), vec(3, 0))),
		hostileRejectedShape("sensor-chain",
			"a chain marked as a sensor (Box2D has no sensor chains)",
			asSensor(chain(vec(-3, 0), vec(-1, 0), vec(1, 0), vec(3, 0)))),
		hostileRejectedShape("zero-radius-circle",
			"a circle of radius 0",
			circle(0)),
		hostileRejectedShape("negative-radius-circle",
			"a circle of radius -1",
			circle(-1)),
		hostileRejectedShape("zero-extent-box",
			"a box with zero half-extents",
			box(0, 0)),
		hostileRejectedShape("polygon-too-many-vertices",
			"a convex polygon of 9 vertices (Box2D's limit is 8)",
			polygon(
				vec(1, 0), vec(0.77, 0.64), vec(0.17, 0.98), vec(-0.5, 0.87),
				vec(-0.94, 0.34), vec(-0.94, -0.34), vec(-0.5, -0.87),
				vec(0.17, -0.98), vec(0.77, -0.64))),
		hostileRejectedShape("polygon-two-vertices",
			"a convex polygon of 2 vertices",
			polygon(vec(-1, 0), vec(1, 0))),
		hostileRejectedShape("polygon-no-vertices",
			"a convex polygon with no vertices at all",
			polygon()),
		hostileRejectedShape("polygon-flat",
			"a polygon whose vertices all lie on one line",
			polygon(vec(-1, 0), vec(0, 0), vec(1, 0), vec(2, 0))),
		hostileRejectedShape("degenerate-capsule",
			"a capsule whose two centers are the same point",
			capsule(vec(0, 0), vec(0, 0), 0.5)),
		hostileRejectedShape("degenerate-edge",
			"an edge whose two endpoints are the same point",
			edge(vec(0, 0), vec(0, 0))),
		hostileBadShape("chain-on-dynamic-body",
			"a chain fixture on a dynamic body, which has no mass",
			physics.BodyTypeDynamic,
			chain(vec(-3, 0), vec(-1, 0), vec(1, 0), vec(3, 0))),
	}
}

// hostileDestroyDuringContact destroys a body while it is touching another one.
//
// Box2D reports the resulting end-of-touch event with shape ids that are already
// dead, and documents that a caller must check b2Shape_IsValid before using
// them. The bridge does exactly that for sensor end events and not for contact
// end events, so fill_contact_event dereferences a freed shape and Box2D's
// assertion kills the shard.
func hostileDestroyDuringContact() harness.Scenario {
	var s struct {
		floor cardinal.EntityID
		ball  cardinal.EntityID
	}
	return harness.Scenario{
		Name: "destroy-during-contact",
		Setup: func(c *harness.Ctx) {
			s.floor = c.Spawn("floor", 0, groundY, body(c, physics.BodyTypeStatic, box(5, 1)))
			s.ball = c.Spawn("ball", 0, 3, body(c, physics.BodyTypeDynamic, circle(0.5)))
		},
		Steps: []harness.Step{
			{Tick: 90, Do: func(c *harness.Ctx) {
				touching := c.CountBetween(harness.ContactBegin, s.ball, s.floor)
				if !c.IntAtLeast("the ball is in contact before it is destroyed", touching, 1) {
					return
				}
				c.Note("destroying entity %d while it is touching entity %d — "+
					"the next step drains a ContactEnd for a shape that no longer exists",
					s.ball, s.floor)
				c.Destroy(s.ball)
			}},
			{Tick: 95, Do: func(c *harness.Ctx) {
				c.True("the shard survives destroying a body that was in contact",
					true, "unreachable")
				c.IntAtLeast("destroying a contacting body still reports ContactEnd",
					c.CountBetween(harness.ContactEnd, s.ball, s.floor), 1)
			}},
		},
	}
}

// hostileBadShape spawns one shape that PhysicsBody2D.Validate accepts (it only checks
// slots; the shape itself is validated at attach) and Box2D may not. The body is created
// mid-run, so the failing attach lands on a world that is already running.
func hostileBadShape(
	name, description string, kind physics.BodyType, shape ShapeSpec,
) harness.Scenario {
	var victim cardinal.EntityID
	return harness.Scenario{
		Name: name,
		Setup: func(c *harness.Ctx) {
			c.Spawn("bystander", 0, 0, body(c, physics.BodyTypeStatic, box(5, 1)))
		},
		Steps: []harness.Step{
			{Tick: 5, Do: func(c *harness.Ctx) {
				pb := body(c, kind, shape)
				c.NoError("PhysicsBody2D.Validate accepts "+description, pb.Validate())
				c.Note("spawning %s", description)
				victim = c.Spawn("victim", 0, 10, pb)
			}},
			{Tick: 20, Do: func(c *harness.Ctx) {
				c.True("the shard survives "+description, true, "unreachable")
				res := c.Raycast(0, 14, 0, 6, nil)
				if res.Hit && res.Entity == victim {
					c.Note("%s was accepted and built a real fixture", description)
				} else {
					c.Note("%s was rejected; the entity exists in ECS with no Box2D "+
						"body, and the plugin logs the failure once per tick "+
						"for as long as the entity lives", description)
				}
			}},
		},
	}
}

// hostileRejectedShape puts a shape Box2D could never build on a body. Validate names the
// problem, the body gets no fixture and logs every tick, and nothing else is disturbed.
func hostileRejectedShape(name, description string, shape ShapeSpec) harness.Scenario {
	var bystander, victim cardinal.EntityID
	return harness.Scenario{
		Name: name,
		Setup: func(c *harness.Ctx) {
			bystander = c.Spawn("bystander", 0, 0, body(c, physics.BodyTypeStatic, box(5, 1)))
		},
		Steps: []harness.Step{
			{Tick: 5, Do: func(c *harness.Ctx) {
				c.Note("spawning a body with %s", description)
				_, err := shape.TrySpawn(c)
				c.HasError("Validate refuses "+description, err)
				victim = c.Spawn("victim", 0, 10, physcomp.NewPhysicsBody2D(physics.BodyTypeStatic, shape.Shape))
			}},
			{Tick: 20, Do: func(c *harness.Ctx) {
				ids, ok := c.Plugin().ShapeIDs(victim)
				c.True("the body gets no fixture", !ok || len(ids) == 0,
					"the rejected shape attached %d fixture(s)", len(ids))
				c.True("the shard survives "+description, true, "unreachable")
				c.True("the bystander still has its fixture",
					c.OverlapHits(c.OverlapAABB(-5, -1, 5, 1, nil), bystander),
					"the bystander lost its body")
			}},
		},
	}
}
