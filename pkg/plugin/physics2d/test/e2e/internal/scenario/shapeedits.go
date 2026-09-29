package scenario

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
)

// ShapeEdits pins what an edit to a body's shapes does to its fixtures: material and filter
// changes land in place, geometry and sensor changes rebuild, a chain's material reaches its
// segments without rebuilding them, and removing a middle shape moves the later fixtures down
// one index.
func ShapeEdits() harness.Scenario {
	var s struct {
		body, terrain, row cardinal.EntityID
		fixture            box2d.ShapeID
		segment            box2d.ShapeID
		segments           int
	}
	edit := func(c *harness.Ctx, id cardinal.EntityID, f func(physics.Shape) physics.Shape) {
		c.EditBody(id, func(pb *physics.PhysicsBody2D) { pb.Shapes = pb.Shapes.With(0, f(pb.Shapes.At(0))) })
	}
	return harness.Scenario{
		Name: "shape-edits",
		Setup: func(c *harness.Ctx) {
			s.body = c.Spawn("editable", 0, 5, body(c, physics.BodyTypeStatic, box(1, 1)))
			s.terrain = c.Spawn("terrain", 0, 0,
				body(c, physics.BodyTypeStatic, chain(vec(-4, 0), vec(-1, 0.5), vec(1, 0.5), vec(4, 0))))
			// Three boxes in a row, fixtures 0, 1 and 2 from left to right.
			s.row = c.Spawn("row", 0, 15, body(c, physics.BodyTypeStatic,
				atOffset(box(1, 1), -3, 0), box(1, 1), atOffset(box(1, 1), 3, 0)))
		},
		Steps: []harness.Step{
			{Tick: 2, Do: func(c *harness.Ctx) {
				s.fixture = fixtureOf(c, s.body)
				c.True("the body has a fixture", !s.fixture.IsNull(), "no fixture")
				s.segment, s.segments = firstSegment(c, s.terrain)
				// An open chain's first and last points are ghosts: four points make one segment.
				c.Int("the chain has one segment", s.segments, 1)
				edit(c, s.body, func(sh physics.Shape) physics.Shape { return sh.Material(0.9, 0.1, 1) })
			}},
			{Tick: 3, Do: func(c *harness.Ctx) {
				c.True("a material edit keeps the fixture", fixtureOf(c, s.body) == s.fixture, "fixture was rebuilt")
				c.Near("and reaches it", c.Plugin().Engine().ShapeFriction(s.fixture), 0.9, 0)
				edit(c, s.body, func(sh physics.Shape) physics.Shape { return sh.Filter(0x8, 0x8) })
			}},
			{Tick: 4, Do: func(c *harness.Ctx) {
				c.True("a filter edit keeps the fixture", fixtureOf(c, s.body) == s.fixture, "fixture was rebuilt")
				c.True("and reaches it", c.Plugin().Engine().ShapeFilter(s.fixture).CategoryBits == 0x8, "filter not applied")
				edit(c, s.body, func(physics.Shape) physics.Shape { return box(2, 1).Spawn(c).Filter(0x8, 0x8) })
			}},
			{Tick: 5, Do: func(c *harness.Ctx) {
				now := fixtureOf(c, s.body)
				c.True("a geometry edit rebuilds the fixture", now != s.fixture && !now.IsNull(), "fixture kept")
				c.True("with the new geometry", c.OverlapHits(c.OverlapAABB(1.5, 4.5, 1.9, 5.5, nil), s.body),
					"the wider box is not there")
				s.fixture = now
				edit(c, s.body, func(sh physics.Shape) physics.Shape { return sh.Sensor(true) })
			}},
			{Tick: 6, Do: func(c *harness.Ctx) {
				now := fixtureOf(c, s.body)
				c.True("a sensor edit rebuilds the fixture", now != s.fixture && !now.IsNull(), "fixture kept")
				c.False("and the body is a sensor now", c.OverlapHits(c.OverlapAABB(-0.5, 4.5, 0.5, 5.5, nil), s.body),
					"a nil filter skips sensors, yet the body was hit")
				edit(c, s.terrain, func(sh physics.Shape) physics.Shape { return sh.Material(0.05, 0, 1) })
			}},
			{Tick: 7, Do: func(c *harness.Ctx) {
				seg, n := firstSegment(c, s.terrain)
				c.True("a chain material edit keeps its segments", seg == s.segment && n == s.segments,
					"segments were rebuilt")
				c.Near("and reaches every segment", c.Plugin().Engine().ShapeFriction(seg), 0.05, 0)
				c.True("the right box starts as fixture 2",
					c.OverlapHitsShape(c.OverlapAABB(2.5, 14.5, 3.5, 15.5, nil), s.row, 2), "no hit at index 2")
				c.EditBody(s.row, func(pb *physics.PhysicsBody2D) { pb.Shapes = pb.Shapes.Without(1) })
			}},
			{Tick: 8, Do: func(c *harness.Ctx) {
				c.Int("Without removes one shape", c.Body(s.row).Shapes.Len(), 2)
				c.False("the removed middle box is gone", c.OverlapHits(c.OverlapAABB(-0.5, 14.5, 0.5, 15.5, nil), s.row),
					"something is still at the middle")
				c.True("the right box moved down to fixture 1",
					c.OverlapHitsShape(c.OverlapAABB(2.5, 14.5, 3.5, 15.5, nil), s.row, 1), "no hit at index 1")
				c.True("the left box is still fixture 0",
					c.OverlapHitsShape(c.OverlapAABB(-3.5, 14.5, -2.5, 15.5, nil), s.row, 0), "no hit at index 0")
			}},
		},
	}
}

// fixtureOf is the Box2D shape behind slot 0 of the entity's body, or a null id.
func fixtureOf(c *harness.Ctx, id cardinal.EntityID) box2d.ShapeID {
	ids, ok := c.Plugin().ShapeIDs(id)
	if !ok || len(ids) == 0 {
		return box2d.ShapeID{}
	}
	return ids[0]
}

// firstSegment is the first Box2D shape on the entity's body (a chain's first segment) and
// how many shapes the body has.
func firstSegment(c *harness.Ctx, id cardinal.EntityID) (box2d.ShapeID, int) {
	bodyID, ok := c.Plugin().BodyID(id)
	if !ok {
		return box2d.ShapeID{}, 0
	}
	w := c.Plugin().Engine()
	ids := make([]box2d.ShapeID, w.BodyShapeCount(bodyID))
	n := w.BodyShapes(bodyID, ids)
	if n == 0 {
		return box2d.ShapeID{}, 0
	}
	return ids[0], n
}
