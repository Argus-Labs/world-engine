package scenario

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Shapes exercises all seven ColliderShape kinds end to end: each one must reach
// the C side (a query finds it), and each must actually collide the way its
// geometry says it should.
//
// The four mass-bearing kinds (circle, box, convex polygon, capsule) are dropped
// onto a static floor and must come to rest at the height their geometry implies.
// The three static-only kinds (open chain, chain loop, edge) are built as world
// geometry and must catch a ball dropped onto them.
//
// Chains in Box2D v3 are one-sided: the surface normal faces to the right of the
// segment direction. Points are therefore ordered right-to-left where an
// upward-facing normal is wanted, and the loop uses the winding that puts the
// normals on the inside.
func Shapes() harness.Scenario {
	var s struct {
		floor       cardinal.EntityID
		ball        cardinal.EntityID
		crate       cardinal.EntityID
		wedge       cardinal.EntityID
		pill        cardinal.EntityID
		chainRail   cardinal.EntityID
		chainBall   cardinal.EntityID
		edgeRail    cardinal.EntityID
		edgeBall    cardinal.EntityID
		loop        cardinal.EntityID
		loopBall    cardinal.EntityID
		capRail     cardinal.EntityID
		capRailBall cardinal.EntityID
	}

	const (
		dropY    = 8.0
		settleAt = 170
	)

	return harness.Scenario{
		Name: "shapes",
		Setup: func(c *harness.Ctx) {
			s.floor = c.Spawn("floor", 0, groundY, ground(60))

			// Mass-bearing shapes, dropped onto the floor at y=0.
			s.ball = c.Spawn("circle", -24, dropY,
				body(physics.BodyTypeDynamic, circle(0.5)))
			s.crate = c.Spawn("box", -18, dropY,
				body(physics.BodyTypeDynamic, box(0.5, 0.5)))
			s.wedge = c.Spawn("convex-polygon", -12, dropY,
				body(physics.BodyTypeDynamic, polygon(
					vec(-0.5, -0.5), vec(0.5, -0.5), vec(0, 0.5))))
			s.pill = c.Spawn("capsule", -6, dropY,
				body(physics.BodyTypeDynamic, capsule(vec(-0.5, 0), vec(0.5, 0), 0.3)))

			// Open chain rail at y=3, points right-to-left for an upward normal.
			// Box2D asserts count >= 4 on any chain, loop or not.
			s.chainRail = c.Spawn("chain-rail", 8, 3,
				body(physics.BodyTypeStatic, chain(
					vec(3, 0), vec(1, 0), vec(-1, 0), vec(-3, 0))))
			s.chainBall = c.Spawn("chain-ball", 8, 9,
				body(physics.BodyTypeDynamic, circle(0.3)))

			// Single edge segment at y=3. Segments are two-sided, so winding is free.
			s.edgeRail = c.Spawn("edge-rail", 16, 3,
				body(physics.BodyTypeStatic, edge(vec(-3, 0), vec(3, 0))))
			s.edgeBall = c.Spawn("edge-ball", 16, 9,
				body(physics.BodyTypeDynamic, circle(0.3)))

			// Closed loop: a sealed 6x6 box centred at (26,4), so its floor is at
			// y=1. A ball started inside must land on that floor and stay inside;
			// if the loop behaved like an open chain it would fall straight out.
			s.loop = c.Spawn("chain-loop", 26, 4,
				body(physics.BodyTypeStatic, chainLoop(
					vec(-3, -3), vec(-3, 3), vec(3, 3), vec(3, -3))))
			s.loopBall = c.Spawn("loop-ball", 26, 6,
				body(physics.BodyTypeDynamic, circle(0.3)))

			// Capsule as static world geometry, not just as a dynamic body.
			s.capRail = c.Spawn("capsule-rail", 34, 3,
				body(physics.BodyTypeStatic, capsule(vec(-2, 0), vec(2, 0), 0.5)))
			s.capRailBall = c.Spawn("capsule-rail-ball", 34, 9,
				body(physics.BodyTypeDynamic, circle(0.3)))
		},
		Steps: []harness.Step{
			{Tick: 3, Do: func(c *harness.Ctx) {
				// Every shape kind must exist on the C side. OverlapAABB runs the
				// narrow phase, so a hit means real geometry, not just an AABB.
				present := []struct {
					id                     cardinal.EntityID
					name                   string
					minX, minY, maxX, maxY float64
				}{
					{s.floor, "box (static floor)", -5, -2, 5, 0},
					{s.ball, "circle", -25, dropY - 1, -23, dropY + 1},
					{s.crate, "box (dynamic)", -19, dropY - 1, -17, dropY + 1},
					{s.wedge, "convex polygon", -13, dropY - 1, -11, dropY + 1},
					{s.pill, "capsule (dynamic)", -7, dropY - 1, -5, dropY + 1},
					{s.chainRail, "static chain", 6, 2.5, 10, 3.5},
					{s.edgeRail, "edge", 14, 2.5, 18, 3.5},
					{s.loop, "chain loop", 22, 0.5, 30, 7.5},
					{s.capRail, "capsule (static)", 32, 2.5, 36, 3.5},
				}
				for _, p := range present {
					hits := c.OverlapAABB(p.minX, p.minY, p.maxX, p.maxY, nil)
					c.True("shape reaches Box2D: "+p.name, c.OverlapHits(hits, p.id),
						"OverlapAABB over the %s found %d hit(s), none of them entity %d",
						p.name, len(hits.Hits), p.id)
				}
			}},
			{Tick: settleAt, Do: func(c *harness.Ctx) {
				// Resting heights. Each body's contact surface sits at its own
				// support height above the floor, so a wrong rest height means the
				// geometry was built wrong even though a body exists.
				rest := []struct {
					id    cardinal.EntityID
					name  string
					wantY float64
					wantX float64
				}{
					{s.ball, "circle rests on its radius", 0.5, -24},
					{s.crate, "box rests on its half-height", 0.5, -18},
					{s.wedge, "convex polygon rests on its base", 0.5, -12},
					{s.pill, "capsule rests on its radius", 0.3, -6},
					{s.chainBall, "ball rests on the open chain", 3.3, 8},
					{s.edgeBall, "ball rests on the edge", 3.3, 16},
					{s.loopBall, "ball rests on the loop floor", 1.3, 26},
					{s.capRailBall, "ball rests on the static capsule", 3.8, 34},
				}
				for _, r := range rest {
					pos := c.Pos(r.id)
					c.Near(r.name, pos.Y, r.wantY, 0.15)
					c.Near(r.name+" without drifting sideways", pos.X, r.wantX, 0.35)
					c.Less(r.name+" and comes to rest", c.Speed(r.id), 0.2)
				}

				// The loop must contain the ball, not merely slow it down.
				c.Greater("chain loop keeps the ball inside", c.Pos(s.loopBall).Y, 0.9)

				// Static geometry must never move, no matter what lands on it.
				statics := []struct {
					id           cardinal.EntityID
					name         string
					wantX, wantY float64
				}{
					{s.floor, "static floor", 0, groundY},
					{s.chainRail, "static chain rail", 8, 3},
					{s.edgeRail, "static edge rail", 16, 3},
					{s.loop, "static chain loop", 26, 4},
					{s.capRail, "static capsule rail", 34, 3},
				}
				for _, st := range statics {
					c.NearVec(st.name+" never moves", c.Pos(st.id), vec(st.wantX, st.wantY), 1e-9)
				}
			}},
		},
	}
}
