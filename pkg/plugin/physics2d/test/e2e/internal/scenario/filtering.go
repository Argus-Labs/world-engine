package scenario

import (
	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Filtering covers collision filtering end to end: category/mask pairs, the
// asymmetric case where only one side agrees, group index overrides, and the
// full 64-bit width of the filter fields.
//
// The width test matters for a port specifically: Box2D v2 used 16-bit filters
// and Box2D v3.1 widened them to uint64_t. A bridge that still marshals them
// through a uint16 or uint32 would behave perfectly for every category below bit
// 32 and silently stop colliding above it.
//
// Every pair is a head-on approach on its own row with gravity switched off, so
// "did they collide" reduces to "did they stop or did they pass through".
func Filtering() harness.Scenario {
	const (
		gap        = 10.0
		closing    = 2.0
		checkTick  = 240
		highBitA   = uint64(1) << 40
		highBitB   = uint64(1) << 41
		queryCat   = uint64(0x8)
		queryOther = uint64(0x10)
	)

	type pair struct {
		left, right cardinal.EntityID
		name        string
		shouldHit   bool
		why         string
	}
	var pairs []*pair
	var queryWall, groupedWall cardinal.EntityID

	// mover builds a zero-gravity box that slides along its row toward the other
	// side of the pair. FixedRotation keeps a glancing hit from spinning it out
	// of its row and into a neighbouring test.
	mover := func(cat, mask uint64, group int32) physics.PhysicsBody2D {
		pb := body(physics.BodyTypeDynamic, withFilter(box(0.5, 0.5), cat, mask, group))
		pb.GravityScale = 0
		pb.FixedRotation = true
		return pb
	}

	// The pair table is declared up front so the setup and the assertions cannot
	// drift apart; Setup fills in the entity IDs.
	pairsSpec := []struct {
		name        string
		why         string
		row         float64
		lCat, lMask uint64
		rCat, rMask uint64
		lGroup      int32
		rGroup      int32
		shouldHit   bool
	}{
		{
			"category/mask agree",
			"each side's mask names the other's category",
			0, 0x1, 0x2, 0x2, 0x1, 0, 0, true,
		},
		{
			"category/mask disagree",
			"neither side's mask names the other's category",
			10, 0x1, 0x2, 0x4, 0x4, 0, 0, false,
		},
		{
			"only one side agrees",
			"Box2D needs both masks to accept; one-sided agreement is not enough",
			20, 0x1, 0xFFFF, 0x2, 0x2, 0, 0, false,
		},
		{
			"same negative group",
			"a shared negative group index vetoes a collision the masks allow",
			30, catAll, maskAll, catAll, maskAll, -5, -5, false,
		},
		{
			"same positive group",
			"a shared positive group index forces a collision the masks forbid",
			40, 0x1, 0x1, 0x2, 0x2, 7, 7, true,
		},
		{
			"matching bit 40",
			"category bits above 32 must survive the trip into Box2D's uint64 filter",
			50, highBitA, highBitA, highBitA, highBitA, 0, 0, true,
		},
		{
			"differing high bits",
			"bit 40 and bit 41 are different categories and must not match",
			60, highBitA, highBitA, highBitB, highBitB, 0, 0, false,
		},
		{
			"different groups fall back to masks",
			"unequal group indices are ignored, so the masks decide",
			70, 0x1, 0x2, 0x2, 0x1, 3, -4, true,
		},
	}

	for _, spec := range pairsSpec {
		pairs = append(pairs, &pair{name: spec.name, shouldHit: spec.shouldHit, why: spec.why})
	}

	return harness.Scenario{
		Name: "filtering",
		Setup: func(c *harness.Ctx) {
			for i, spec := range pairsSpec {
				p := pairs[i]
				p.left = c.SpawnMoving(spec.name+"/left", 0, spec.row, closing, 0,
					mover(spec.lCat, spec.lMask, spec.lGroup))
				p.right = c.SpawnMoving(spec.name+"/right", gap, spec.row, -closing, 0,
					mover(spec.rCat, spec.rMask, spec.rGroup))
			}

			// Query filtering targets, parked well away from the moving pairs.
			queryWall = c.Spawn("query-wall", 0, 100,
				body(physics.BodyTypeStatic, withFilter(box(1, 1), queryCat, maskAll, 0)))
			groupedWall = c.Spawn("grouped-wall", 10, 100,
				body(physics.BodyTypeStatic, withFilter(box(1, 1), queryCat, maskAll, -5)))
		},
		Steps: []harness.Step{
			{Tick: 3, Do: func(c *harness.Ctx) {
				// Query filters use the same category/mask handshake as contacts.
				hit := c.Raycast(0, 105, 0, 95, &physics.Filter{
					CategoryBits: maskAll, MaskBits: queryCat,
				})
				c.True("query filter matching the fixture's category hits", hit.Hit,
					"raycast with MaskBits=%#x missed a wall with CategoryBits=%#x",
					queryCat, queryCat)

				miss := c.Raycast(0, 105, 0, 95, &physics.Filter{
					CategoryBits: maskAll, MaskBits: queryOther,
				})
				c.False("query filter naming another category misses", miss.Hit,
					"raycast with MaskBits=%#x hit a wall with CategoryBits=%#x",
					queryOther, queryCat)

				rejected := c.Raycast(0, 105, 0, 95, &physics.Filter{
					CategoryBits: 0, MaskBits: maskAll,
				})
				c.False("query the fixture's own mask rejects misses", rejected.Hit,
					"a query with CategoryBits=0 was still accepted by the fixture's mask")

				// The plugin documents that group index is not part of v1 query
				// filtering. Pin that: a walled-off group must stay findable.
				grouped := c.OverlapAABB(9, 99, 11, 101, nil)
				c.True("queries ignore group index", c.OverlapHits(grouped, groupedWall),
					"a wall with GroupIndex=-5 was not returned by an unfiltered overlap")

				plain := c.OverlapAABB(-1, 99, 1, 101, nil)
				c.True("unfiltered overlap finds the query wall",
					c.OverlapHits(plain, queryWall), "unfiltered overlap missed the wall")

				filtered := c.OverlapAABB(-1, 99, 1, 101, &physics.Filter{
					CategoryBits: maskAll, MaskBits: queryOther,
				})
				c.False("filtered overlap excludes the wrong category",
					c.OverlapHits(filtered, queryWall),
					"overlap with MaskBits=%#x still returned a CategoryBits=%#x wall",
					queryOther, queryCat)
			}},
			{Tick: checkTick, Do: func(c *harness.Ctx) {
				for _, p := range pairs {
					leftX := c.Pos(p.left).X
					rightX := c.Pos(p.right).X
					contacts := c.CountBetween(harness.ContactBegin, p.left, p.right)

					if p.shouldHit {
						c.IntAtLeast(p.name+": bodies collide", contacts, 1)
						c.Less(p.name+": bodies never pass through each other",
							leftX, rightX)
						c.Less(p.name+": left body is stopped by the impact",
							c.Speed(p.left), closing)
					} else {
						c.Int(p.name+": bodies raise no contact", contacts, 0)
						c.Greater(p.name+": bodies pass through each other",
							leftX-rightX, 0)
						c.Near(p.name+": left body keeps its speed unchanged",
							c.Speed(p.left), closing, 1e-3)
					}
					c.Note("%s (%s): left x=%.2f right x=%.2f contacts=%d",
						p.name, p.why, leftX, rightX, contacts)
				}
			}},
		},
	}
}
