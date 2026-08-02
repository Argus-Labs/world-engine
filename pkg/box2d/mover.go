// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file
// src/mover.c plus the b2CollisionPlane/b2PlaneSolverResult declarations from
// include/box2d/collision.h.
//
// This file was missed by the original staged port and discovered by the
// oracle-test sweep: CastMover/CollideMover gather collision planes, but the
// solver that consumes them (upstream b2SolvePlanes/b2ClipVector, the core of
// the character mover documented in docs/character.md) had no Go counterpart.

package box2d

// CollisionPlane is a collision plane that can be fed to SolvePlanes.
// Normally this is assembled by the user from plane results produced by
// CollideMover (upstream b2CollisionPlane).
type CollisionPlane struct {
	// Plane is the collision plane between the mover and some shape.
	Plane Plane

	// PushLimit caps the push applied by this plane. Setting it to a huge
	// value makes the plane as rigid as possible; lower values make the
	// collision soft. Usually in meters.
	PushLimit float64

	// Push is the push on the mover determined by SolvePlanes. Usually in
	// meters.
	Push float64

	// ClipVelocity indicates whether ClipVector should clip against this
	// plane. Should be false for soft collision.
	ClipVelocity bool
}

// PlaneSolverResult is returned by SolvePlanes (upstream b2PlaneSolverResult).
type PlaneSolverResult struct {
	// Translation is the translation of the mover.
	Translation Vec2

	// IterationCount is the number of iterations used by the plane solver.
	// For diagnostics.
	IterationCount int
}

// SolvePlanes solves the position of a mover that satisfies the given
// collision planes (upstream b2SolvePlanes). targetDelta is the desired
// movement from the position used to generate the collision planes. The
// planes' Push fields are written in place.
func SolvePlanes(targetDelta Vec2, planes []CollisionPlane) PlaneSolverResult {
	count := len(planes)
	for i := range count {
		planes[i].Push = 0.0
	}

	delta := targetDelta
	tolerance := LinearSlop

	iteration := 0
	for range 20 {
		totalPush := 0.0
		for planeIndex := range count {
			plane := &planes[planeIndex]

			// Add slop to prevent jitter.
			separation := PlaneSeparation(plane.Plane, delta) + LinearSlop

			push := -separation

			// Clamp accumulated push.
			accumulatedPush := plane.Push
			plane.Push = clampFloat(plane.Push+push, 0.0, plane.PushLimit)
			push = plane.Push - accumulatedPush
			delta = MulAdd(delta, push, plane.Plane.Normal)

			// Track maximum push for convergence.
			totalPush += absFloat(push)
		}

		if totalPush < tolerance {
			break
		}
		iteration++
	}

	return PlaneSolverResult{
		Translation:    delta,
		IterationCount: iteration,
	}
}

// ClipVector clips the velocity against the given collision planes (upstream
// b2ClipVector). Planes with zero push or ClipVelocity set to false are
// skipped.
func ClipVector(vector Vec2, planes []CollisionPlane) Vec2 {
	v := vector

	for planeIndex := range planes {
		plane := &planes[planeIndex]
		if plane.Push == 0.0 || !plane.ClipVelocity {
			continue
		}

		v = MulSub(v, minFloat(0.0, Dot(v, plane.Plane.Normal)), plane.Plane.Normal)
	}

	return v
}
