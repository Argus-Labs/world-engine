package scenario

import (
	"math"

	"github.com/argus-labs/world-engine/pkg/plugin/physics2d/test/e2e/internal/harness"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	physics "github.com/argus-labs/world-engine/pkg/plugin/physics2d"
)

// Queries covers the three query entry points the plugin exposes — Raycast,
// OverlapAABB and CircleSweep — including the documented edge cases (zero-length
// rays, zero-area boxes, zero-radius sweeps, reversed AABB corners) and the
// sensor rule that all three share.
//
// The scene is static and never moves, so every expected value below is exact
// geometry rather than a simulated outcome.
func Queries() harness.Scenario {
	var s struct {
		nearWall   cardinal.EntityID
		farWall    cardinal.EntityID
		sensorWall cardinal.EntityID
		solidWall  cardinal.EntityID
		boxes      []cardinal.EntityID
		sweepWall  cardinal.EntityID
		post       cardinal.EntityID
		plank      cardinal.EntityID
	}

	const (
		rayLen    = 20.0
		nearWallX = 5.0
		farWallX  = 10.0
		wallHalf  = 0.5
		sweepR    = 0.5
		postY     = 30.8
		postR     = 0.1
		plankY    = 40.0
	)

	return harness.Scenario{
		Name: "queries",
		Setup: func(c *harness.Ctx) {
			// Row y=0 — two walls in a line; a ray must stop at the first.
			s.nearWall = c.Spawn("near-wall", nearWallX, 0,
				body(physics.BodyTypeStatic, box(wallHalf, 2)))
			s.farWall = c.Spawn("far-wall", farWallX, 0,
				body(physics.BodyTypeStatic, box(wallHalf, 2)))

			// Row y=10 — a sensor in front of a solid wall.
			s.sensorWall = c.Spawn("sensor-wall", nearWallX, 10,
				body(physics.BodyTypeStatic, asSensor(box(wallHalf, 2))))
			s.solidWall = c.Spawn("solid-wall", farWallX, 10,
				body(physics.BodyTypeStatic, box(wallHalf, 2)))

			// Row y=20 — three separate boxes for the overlap tests.
			for _, x := range []float64{-4, 0, 4} {
				s.boxes = append(s.boxes, c.Spawn("overlap-box", x, 20,
					body(physics.BodyTypeStatic, box(0.5, 0.5))))
			}

			// Row y=30 — a wall to sweep into, plus a thin post parked just off
			// the ray's line so a sweep can find what a ray cannot.
			s.sweepWall = c.Spawn("sweep-wall", farWallX, 30,
				body(physics.BodyTypeStatic, box(wallHalf, 2)))
			s.post = c.Spawn("offset-post", nearWallX, postY,
				body(physics.BodyTypeStatic, circle(postR)))

			// Row y=40 — a thin plank turned 45 degrees. Its axis-aligned
			// bounding box covers a lot of empty space, which is what the
			// broad-phase-versus-narrow-phase check below is about.
			s.plank = c.Spawn("diagonal-plank", 0, plankY,
				body(physics.BodyTypeStatic, rotatedBy(box(2, 0.1), math.Pi/4)))
		},
		Steps: []harness.Step{
			{Tick: 3, Do: func(c *harness.Ctx) {
				checkRaycast(c, s.nearWall, s.farWall, rayLen, nearWallX, wallHalf)
				checkRaycastSensors(c, s.sensorWall, s.solidWall, rayLen, nearWallX, farWallX, wallHalf)
				checkOverlapAndSweepSensors(c, s.sensorWall, s.solidWall, nearWallX, farWallX, wallHalf)
				checkOverlap(c, s.boxes)
				checkSweep(c, s.sweepWall, s.post, rayLen, farWallX, wallHalf, sweepR, postY, postR)
				checkOverlapNarrowPhase(c, s.plank, plankY)
			}},
		},
	}
}

func checkRaycast(c *harness.Ctx, near, far cardinal.EntityID, rayLen, nearWallX, wallHalf float64) {
	surface := nearWallX - wallHalf

	hit := c.Raycast(0, 0, rayLen, 0, nil)
	if !c.True("raycast finds a wall in its path", hit.Hit, "the ray reported no hit") {
		return
	}
	c.True("raycast returns the closest wall, not the furthest", hit.Entity == near,
		"hit entity %d (the far wall is %d)", hit.Entity, far)
	c.Int("raycast reports the struck shape slot", hit.ShapeIndex, 0)
	c.Near("raycast fraction matches the distance to the surface",
		hit.Fraction, surface/rayLen, 1e-3)
	c.NearVec("raycast point lands on the wall's near face in world space",
		hit.Point, vec(surface, 0), 0.02)
	c.NearVec("raycast normal faces back along the ray", hit.Normal, vec(-1, 0), 1e-3)

	// Same segment, cast the other way: the far wall becomes the near one.
	back := c.Raycast(rayLen, 0, 0, 0, nil)
	c.True("raycast from the other side finds the other wall",
		back.Hit && back.Entity == far, "reversed ray hit entity %d, want %d",
		back.Entity, far)
	c.NearVec("reversed raycast normal also faces back along the ray",
		back.Normal, vec(1, 0), 1e-3)

	miss := c.Raycast(0, 50, rayLen, 50, nil)
	c.False("raycast through empty space misses", miss.Hit,
		"ray hit entity %d at y=50 where nothing was placed", miss.Entity)
	c.Near("a missing raycast zeroes its fraction", miss.Fraction, 0, 0)

	zero := c.Raycast(0, 0, 0, 0, nil)
	c.False("a zero-length raycast never hits", zero.Hit,
		"a degenerate ray reported a hit on entity %d", zero.Entity)
}

func checkRaycastSensors(
	c *harness.Ctx, sensorWall, solidWall cardinal.EntityID,
	rayLen, nearWallX, farWallX, wallHalf float64,
) {
	solid := c.Raycast(0, 10, rayLen, 10, nil)
	c.True("raycast skips sensors by default",
		solid.Hit && solid.Entity == solidWall,
		"default ray hit entity %d; it should have passed through the sensor to %d",
		solid.Entity, solidWall)
	c.Near("raycast that skipped a sensor lands on the solid wall behind it",
		solid.Point.X, farWallX-wallHalf, 0.02)

	withSensors := c.Raycast(0, 10, rayLen, 10, &physics.Filter{
		CategoryBits: maskAll, MaskBits: maskAll, IncludeSensors: true,
	})
	c.True("IncludeSensors makes a raycast see sensors",
		withSensors.Hit && withSensors.Entity == sensorWall,
		"ray with IncludeSensors hit entity %d, want the sensor %d",
		withSensors.Entity, sensorWall)
	c.Near("the sensor hit is in front of the solid wall",
		withSensors.Point.X, nearWallX-wallHalf, 0.02)
}

func checkOverlap(c *harness.Ctx, boxes []cardinal.EntityID) {
	all := c.OverlapAABB(-5, 19, 5, 21, nil)
	for _, id := range boxes {
		c.True("overlap finds every box inside the query box",
			c.OverlapHits(all, id), "entity %d was not returned", id)
	}
	c.Int("overlap returns one hit per shape, with no duplicates", len(all.Hits), len(boxes))

	// Reversed corners are documented as equivalent, per-axis.
	swapped := c.OverlapAABB(5, 21, -5, 19, nil)
	c.Int("overlap accepts reversed corners", len(swapped.Hits), len(all.Hits))

	partial := c.OverlapAABB(-5, 19, -3, 21, nil)
	c.Int("overlap excludes boxes outside the query box", len(partial.Hits), 1)
	c.True("overlap returns the box that is inside", c.OverlapHits(partial, boxes[0]),
		"the leftmost box was not the one returned")

	flatX := c.OverlapAABB(0, 19, 0, 21, nil)
	c.Int("a zero-width overlap box returns nothing", len(flatX.Hits), 0)

	flatY := c.OverlapAABB(-5, 20, 5, 20, nil)
	c.Int("a zero-height overlap box returns nothing", len(flatY.Hits), 0)

	empty := c.OverlapAABB(-5, 49, 5, 51, nil)
	c.Int("an overlap over empty space returns nothing", len(empty.Hits), 0)
}

// checkOverlapAndSweepSensors pins the sensor rule for the other two query kinds.
// A sensor reports overlaps without blocking anything, so every query skips sensors
// unless Filter.IncludeSensors says otherwise — the same rule checkRaycastSensors
// pins for rays, which is easy to implement for one query and forget for the rest.
func checkOverlapAndSweepSensors(
	c *harness.Ctx, sensorWall, solidWall cardinal.EntityID,
	sensorX, solidX, wallHalf float64,
) {
	withSensors := &physics.Filter{
		CategoryBits: ^uint64(0), MaskBits: ^uint64(0), IncludeSensors: true,
	}

	// A box spanning both walls: by default only the solid one comes back.
	minX, maxX := sensorX-wallHalf-1, solidX+wallHalf+1
	byDefault := c.OverlapAABB(minX, 8, maxX, 12, nil)
	c.True("overlap skips sensors by default", c.OverlapHits(byDefault, solidWall),
		"the solid wall was not returned")
	c.False("overlap excludes the sensor by default", c.OverlapHits(byDefault, sensorWall),
		"the sensor wall was returned without IncludeSensors")

	included := c.OverlapAABB(minX, 8, maxX, 12, withSensors)
	c.True("IncludeSensors makes an overlap see sensors", c.OverlapHits(included, sensorWall),
		"the sensor wall was not returned with IncludeSensors")
	c.True("IncludeSensors keeps solid hits too", c.OverlapHits(included, solidWall),
		"the solid wall was dropped when sensors were included")

	// A sweep along the same row hits the sensor first only when asked to.
	startX := sensorX - wallHalf - 5
	endX := solidX + wallHalf
	plain := c.CircleSweep(startX, 10, endX, 10, 0.4, 0, nil)
	c.True("circle sweep skips sensors by default", plain.Hit && plain.Entity == solidWall,
		"default sweep hit entity %d; it should have passed through the sensor to %d",
		plain.Entity, solidWall)

	withSensor := c.CircleSweep(startX, 10, endX, 10, 0.4, 0, withSensors)
	c.True("IncludeSensors makes a circle sweep see sensors",
		withSensor.Hit && withSensor.Entity == sensorWall,
		"sweep with IncludeSensors hit entity %d, want the sensor %d",
		withSensor.Entity, sensorWall)
}

func checkSweep(
	c *harness.Ctx, wall, post cardinal.EntityID,
	rayLen, wallX, wallHalf, radius, postY, postR float64,
) {
	// The sweep centre stops one radius short of the wall face.
	wantFraction := (wallX - wallHalf - radius) / rayLen

	hit := c.CircleSweep(0, 30, rayLen, 30, radius, 0, nil)
	if !c.True("circle sweep finds the wall in its path", hit.Hit, "the sweep reported no hit") {
		return
	}
	c.True("circle sweep returns the wall", hit.Entity == wall,
		"sweep hit entity %d, want %d", hit.Entity, wall)
	c.Near("circle sweep fraction accounts for the sweeping radius",
		hit.Fraction, wantFraction, 2e-3)
	c.NearVec("circle sweep normal faces back along the sweep", hit.Normal, vec(-1, 0), 1e-2)

	capped := c.CircleSweep(0, 30, rayLen, 30, radius, wantFraction/2, nil)
	c.False("MaxFraction stops a sweep short of a hit beyond it", capped.Hit,
		"a sweep capped at %.3f still hit at %.3f", wantFraction/2, capped.Fraction)

	full := c.CircleSweep(0, 30, rayLen, 30, radius, 1, nil)
	c.True("MaxFraction of 1 sweeps the whole segment", full.Hit,
		"the full-length sweep found nothing")

	noRadius := c.CircleSweep(0, 30, rayLen, 30, 0, 0, nil)
	c.False("a zero-radius sweep never hits", noRadius.Hit,
		"a zero-radius sweep reported a hit on entity %d", noRadius.Entity)

	zeroLen := c.CircleSweep(0, 30, 0, 30, radius, 0, nil)
	c.False("a zero-length sweep never hits", zeroLen.Hit,
		"a degenerate sweep reported a hit on entity %d", zeroLen.Entity)

	// A fat sweep reaches things a thin ray misses; that is the whole reason the
	// call exists, so prove both halves on the same geometry.
	ray := c.Raycast(0, 30, rayLen, 30, nil)
	c.True("a ray along the sweep's line misses the offset post",
		ray.Hit && ray.Entity != post,
		"the ray hit entity %d; the post at y=%.1f should be out of its reach",
		ray.Entity, postY)

	fat := c.CircleSweep(0, 30, rayLen, 30, 1.0, 0, nil)
	c.True("a wide sweep does reach the offset post",
		fat.Hit && fat.Entity == post,
		"the wide sweep hit entity %d, want the post %d", fat.Entity, post)

	// Contact at centre distance radius+postR, offset postY-30 vertically.
	dy := postY - 30
	dx := math.Sqrt((1.0+postR)*(1.0+postR) - dy*dy)
	c.Near("the wide sweep stops where it first touches the post",
		fat.Fraction, (5.0-dx)/rayLen, 5e-3)
}

func checkOverlapNarrowPhase(c *harness.Ctx, plank cardinal.EntityID, plankY float64) {
	// The plank runs along the +x/+y diagonal. This probe sits in the opposite
	// corner of its bounding box, where there is no geometry at all.
	const probeX, probeDY = 1.4, -1.4
	y := plankY + probeDY

	ray := c.Raycast(probeX, y+0.4, probeX, y-0.4, nil)
	c.False("nothing is really at the corner of the plank's bounding box",
		ray.Hit && ray.Entity == plank,
		"a ray through the probe point hit the plank, so the point is not empty "+
			"and this check cannot say anything about the overlap query")

	hits := c.OverlapAABB(probeX-0.05, y-0.05, probeX+0.05, y+0.05, nil)
	c.False("OverlapAABB reports only shapes it actually overlaps",
		c.OverlapHits(hits, plank),
		"OverlapAABB returned the plank for a probe at (%.2f, %.2f), where a "+
			"raycast proves there is no geometry. The bridge's overlap callback "+
			"reports every shape whose broad-phase AABB overlaps the query box "+
			"and runs no narrow-phase test, but AABBOverlapHit is documented as "+
			"a shape that overlaps \"after narrow-phase test\"", probeX, y)

	// The plank must still be found where it really is.
	onPlank := c.OverlapAABB(1.35, plankY+1.35, 1.45, plankY+1.45, nil)
	c.True("OverlapAABB finds the plank where the plank actually is",
		c.OverlapHits(onPlank, plank), "a probe on the plank's diagonal missed it")
}
