// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/aabb.c, src/aabb.h.

package box2d

import "math"

// IsValidAABB reports whether the AABB is well formed and free of NaN/inf
// (upstream b2IsValidAABB).
func IsValidAABB(a AABB) bool {
	d := Sub(a.UpperBound, a.LowerBound)
	valid := d.X >= 0.0 && d.Y >= 0.0
	valid = valid && IsValidVec2(a.LowerBound) && IsValidVec2(a.UpperBound)
	return valid
}

// AABBRayCast casts a ray against an AABB (upstream b2AABB_RayCast).
//
// From Real-time Collision Detection, p179.
func AABBRayCast(a AABB, p1, p2 Vec2) CastOutput {
	// Radius not handled
	var output CastOutput

	tmin := -math.MaxFloat64
	tmax := math.MaxFloat64

	p := p1
	d := Sub(p2, p1)
	absD := Abs(d)

	normal := Vec2Zero

	// x-coordinate
	if absD.X < epsilon {
		// parallel
		if p.X < a.LowerBound.X || a.UpperBound.X < p.X {
			return output
		}
	} else {
		invD := 1.0 / d.X
		t1 := (a.LowerBound.X - p.X) * invD
		t2 := (a.UpperBound.X - p.X) * invD

		// Sign of the normal vector.
		s := -1.0

		if t1 > t2 {
			t1, t2 = t2, t1
			s = 1.0
		}

		// Push the min up
		if t1 > tmin {
			normal.Y = 0.0
			normal.X = s
			tmin = t1
		}

		// Pull the max down
		tmax = minFloat(tmax, t2)

		if tmin > tmax {
			return output
		}
	}

	// y-coordinate
	if absD.Y < epsilon {
		// parallel
		if p.Y < a.LowerBound.Y || a.UpperBound.Y < p.Y {
			return output
		}
	} else {
		invD := 1.0 / d.Y
		t1 := (a.LowerBound.Y - p.Y) * invD
		t2 := (a.UpperBound.Y - p.Y) * invD

		// Sign of the normal vector.
		s := -1.0

		if t1 > t2 {
			t1, t2 = t2, t1
			s = 1.0
		}

		// Push the min up
		if t1 > tmin {
			normal.X = 0.0
			normal.Y = s
			tmin = t1
		}

		// Pull the max down
		tmax = minFloat(tmax, t2)

		if tmin > tmax {
			return output
		}
	}

	// Does the ray start inside the box?
	if tmin < 0.0 {
		return output
	}

	// Does the ray intersect beyond the segment length?
	if 1.0 < tmin {
		return output
	}

	// Intersection.
	output.Fraction = tmin
	output.Normal = normal
	output.Point = Lerp(p1, p2, tmin)
	output.Hit = true
	return output
}

// perimeter returns the surface area of an AABB, i.e. the perimeter length
// (upstream b2Perimeter).
func perimeter(a AABB) float64 {
	wx := a.UpperBound.X - a.LowerBound.X
	wy := a.UpperBound.Y - a.LowerBound.Y
	return 2.0 * (wx + wy)
}

// enlargeAABB enlarges a to contain b and reports whether the AABB grew
// (upstream b2EnlargeAABB).
func enlargeAABB(a *AABB, b AABB) bool {
	changed := false
	if b.LowerBound.X < a.LowerBound.X {
		a.LowerBound.X = b.LowerBound.X
		changed = true
	}

	if b.LowerBound.Y < a.LowerBound.Y {
		a.LowerBound.Y = b.LowerBound.Y
		changed = true
	}

	if a.UpperBound.X < b.UpperBound.X {
		a.UpperBound.X = b.UpperBound.X
		changed = true
	}

	if a.UpperBound.Y < b.UpperBound.Y {
		a.UpperBound.Y = b.UpperBound.Y
		changed = true
	}

	return changed
}
