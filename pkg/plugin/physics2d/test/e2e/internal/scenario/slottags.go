package scenario

import (
	"github.com/argus-labs/world-engine/pkg/box2d"
	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"
)

// SlotTags covers editing a body's slots by name. A tagged slot can be removed or replaced
// without knowing its index, replacing keeps the index (so the fixture survives), adding a
// used tag or touching a missing one fails, and a contact event's slot index maps back to
// the tag.
func SlotTags() harness.Scenario {
	var s struct {
		body     cardinal.EntityID
		ball     cardinal.EntityID
		left     physics.ShapeSlot
		rightFix box2d.ShapeID
	}

	const (
		beforeTick  = 3
		removeTick  = 5
		replaceTick = 8
		afterTick   = 11
		appendTick  = 14
		contactTick = 30
	)

	// addSlot is AddSlot with the error reported as a check.
	addSlot := func(c *harness.Ctx, pb physics.PhysicsBody2D, tag string, slot physics.ShapeSlot) physics.PhysicsBody2D {
		out, err := pb.AddSlot(tag, slot)
		c.NoError("AddSlot "+tag, err)
		return out
	}
	fixture := func(c *harness.Ctx, id cardinal.EntityID, i int) box2d.ShapeID {
		ids, ok := c.Plugin().ShapeIDs(id)
		if !ok || i >= len(ids) {
			return box2d.ShapeID{}
		}
		return ids[i]
	}
	hitsSlot := func(c *harness.Ctx, x, y float64, i int) bool {
		return c.OverlapHitsShape(c.OverlapAABB(x-0.5, y-0.5, x+0.5, y+0.5, nil), s.body, i)
	}

	return harness.Scenario{
		Name: "slot-tags",
		Setup: func(c *harness.Ctx) {
			pb := physics.NewPhysicsBody2D(physics.BodyTypeStatic)
			pb = addSlot(c, pb, "left", box(1, 1).Spawn(c).At(vec(-3, 0), 0))
			pb = addSlot(c, pb, "mid", box(1, 1).Spawn(c))
			pb = addSlot(c, pb, "right", box(1, 1).Spawn(c).At(vec(3, 0), 0))
			s.left = pb.Shapes.At(0)
			_, err := pb.AddSlot("mid", box(1, 1).Spawn(c))
			c.HasError("AddSlot refuses a tag the body already uses", err)
			s.body = c.Spawn("tagged", 0, 0, pb)
		},
		Steps: []harness.Step{
			{Tick: beforeTick, Do: func(c *harness.Ctx) {
				pb := c.Body(s.body)
				c.Int("SlotIndex finds a tagged slot", pb.SlotIndex("mid"), 1)
				c.Int("SlotIndex reports -1 for an unknown tag", pb.SlotIndex("nope"), -1)
				c.Int("SlotIndex reports -1 for the empty tag", pb.SlotIndex(""), -1)
				c.Str("SlotTag names a slot by index", pb.SlotTag(2), "right")
				c.Str("SlotTag is empty out of range", pb.SlotTag(3), "")
				c.True("the right box is fixture 2", hitsSlot(c, 3, 0, 2), "no hit at slot 2")
				s.rightFix = fixture(c, s.body, 2)
			}},
			{Tick: removeTick, Do: func(c *harness.Ctx) {
				pb := c.Body(s.body)
				_, err := pb.RemoveSlot("nope")
				c.HasError("RemoveSlot refuses an unknown tag", err)
				_, err = pb.ReplaceSlot("nope", box(1, 1).Spawn(c))
				c.HasError("ReplaceSlot refuses an unknown tag", err)
				pb, err = pb.RemoveSlot("left")
				if c.NoError("RemoveSlot drops a tagged slot", err) {
					c.SetBody(s.body, pb)
				}
			}},
			{Tick: replaceTick, Do: func(c *harness.Ctx) {
				pb := c.Body(s.body)
				c.Int("RemoveSlot removes one slot", pb.Shapes.Len(), 2)
				c.Int("later slots move down one index", pb.SlotIndex("right"), 1)
				c.False("the removed slot's fixture is gone", hitsSlot(c, -3, 0, 0) || hitsSlot(c, -3, 0, 1), "hit")
				c.True("the right box is now fixture 1", hitsSlot(c, 3, 0, 1), "no hit at slot 1")
				c.False("the removed slot's shape is swept once no body names it",
					c.ShapeAlive(s.left.Shape), "shape entity %d survived", s.left.Shape)
				s.rightFix = fixture(c, s.body, 1)

				// Replace right in place with a rougher box. Alone in its tick: a length change
				// would rebuild every fixture, which is what the next check must not see.
				rough := withFriction(box(1, 1), 0.9).Spawn(c).At(vec(3, 0), 0)
				replaced, err := pb.ReplaceSlot("right", rough)
				if c.NoError("ReplaceSlot swaps a tagged slot", err) {
					c.SetBody(s.body, replaced)
				}
			}},
			{Tick: afterTick, Do: func(c *harness.Ctx) {
				pb := c.Body(s.body)
				c.Int("ReplaceSlot keeps the index", pb.SlotIndex("right"), 1)
				c.Str("ReplaceSlot stamps the tag onto the slot", pb.Shapes.At(1).Tag, "right")
				c.True("a same-geometry replacement keeps its fixture",
					fixture(c, s.body, 1) == s.rightFix, "the fixture was rebuilt")
				c.Near("the replacement's material reached the engine",
					c.Plugin().Engine().ShapeFriction(s.rightFix), 0.9, 0)

				c.SetBody(s.body, addSlot(c, pb, "top", box(1, 1).Spawn(c).At(vec(0, 3), 0)))
			}},
			{Tick: appendTick, Do: func(c *harness.Ctx) {
				pb := c.Body(s.body)
				c.Int("AddSlot appends under the new tag", pb.SlotIndex("top"), 2)
				c.Int("the appended slot grows the list", pb.Shapes.Len(), 3)
				c.True("the appended slot is fixture 2", hitsSlot(c, 0, 3, 2), "no hit at slot 2")

				s.ball = c.Spawn("ball", 0, 4.6, physics.NewPhysicsBody2D(physics.BodyTypeDynamic, circle(0.5).Spawn(c)))
			}},
			{Tick: contactTick, Do: func(c *harness.Ctx) {
				hits := c.EventsBetween(harness.ContactBegin, s.ball, s.body)
				if c.IntAtLeast("the ball lands on the tagged body", len(hits), 1) {
					idx, ok := hits[0].ShapeIndexFor(s.body)
					c.True("the event reports the body's slot", ok, "no slot index for the body")
					c.Str("SlotTag maps a contact's slot index to its name", c.Body(s.body).SlotTag(idx), "top")
				}
			}},
		},
	}
}
