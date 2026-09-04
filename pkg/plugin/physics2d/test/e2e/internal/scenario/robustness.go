package scenario

import (
	"sort"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Robustness holds the inputs a game can hand the plugin that are finite —
// and so pass ColliderShape.Validate — but malformed by the engine's rules: a
// chain with three points, a circle with no radius, a polygon with too many
// vertices, a box with no extent. Destroying an entity that still holds a live
// contact is here too, from a completely different direction.
//
// Validate only checks that numbers are finite, so all of these reach the
// engine. Against the cgo bridge four of them tripped a fatal Box2D assertion
// and killed the shard; the pure-Go engine does not die, which is exactly what
// these cases exist to keep measuring.
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
		hostileBadShape("short-chain",
			"a chain of 3 points (Box2D asserts count >= 4)",
			physics.BodyTypeStatic,
			chain(vec(-3, 0), vec(0, 0), vec(3, 0))),
		hostileBadShape("short-chain-loop",
			"a chain loop of 3 points (Box2D asserts count >= 4)",
			physics.BodyTypeStatic,
			chainLoop(vec(-3, 0), vec(0, 3), vec(3, 0))),
		hostileBadShape("zero-radius-circle",
			"a circle of radius 0",
			physics.BodyTypeDynamic,
			circle(0)),
		hostileBadShape("negative-radius-circle",
			"a circle of radius -1",
			physics.BodyTypeDynamic,
			circle(-1)),
		hostileBadShape("zero-extent-box",
			"a box with zero half-extents",
			physics.BodyTypeDynamic,
			box(0, 0)),
		hostileBadShape("polygon-too-many-vertices",
			"a convex polygon of 9 vertices (Box2D's limit is 8)",
			physics.BodyTypeDynamic,
			polygon(
				vec(1, 0), vec(0.77, 0.64), vec(0.17, 0.98), vec(-0.5, 0.87),
				vec(-0.94, 0.34), vec(-0.94, -0.34), vec(-0.5, -0.87),
				vec(0.17, -0.98), vec(0.77, -0.64))),
		hostileBadShape("polygon-two-vertices",
			"a convex polygon of 2 vertices",
			physics.BodyTypeDynamic,
			polygon(vec(-1, 0), vec(1, 0))),
		hostileBadShape("polygon-no-vertices",
			"a convex polygon with no vertices at all",
			physics.BodyTypeDynamic,
			polygon()),
		hostileBadShape("degenerate-capsule",
			"a capsule whose two centers are the same point",
			physics.BodyTypeDynamic,
			capsule(vec(0, 0), vec(0, 0), 0.5)),
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
			s.floor = c.Spawn("floor", 0, groundY, body(physics.BodyTypeStatic, box(5, 1)))
			s.ball = c.Spawn("ball", 0, 3, body(physics.BodyTypeDynamic, circle(0.5)))
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

// hostileBadShape spawns one shape that ColliderShape.Validate accepts and
// Box2D may not. The body is created mid-run rather than at Init because
// InitPhysicsSystem panics on any FullRebuildFromECS error, which would hide
// which shape was at fault behind a stack trace for the whole scene.
func hostileBadShape(
	name, description string, kind physics.BodyType, shape physics.ColliderShape,
) harness.Scenario {
	var victim cardinal.EntityID
	return harness.Scenario{
		Name: name,
		Setup: func(c *harness.Ctx) {
			c.Spawn("bystander", 0, 0, body(physics.BodyTypeStatic, box(5, 1)))
		},
		Steps: []harness.Step{
			{Tick: 5, Do: func(c *harness.Ctx) {
				pb := body(kind, shape)
				c.NoError("ColliderShape.Validate accepts "+description, pb.Validate())
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
