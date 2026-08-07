// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/hull.c.

package box2d

import "math"

// recurseHull is the quickhull recursion (upstream b2RecurseHull). The C
// version takes a pointer plus a count; the Go version takes the equivalent
// slice, so count is len(ps).
func recurseHull(p1, p2 Vec2, ps []Vec2) Hull {
	var hull Hull
	hull.Count = 0

	count := len(ps)
	if count == 0 {
		return hull
	}

	// create an edge vector pointing from p1 to p2
	e := Normalize(Sub(p2, p1))

	// discard points left of e and find point furthest to the right of e
	var rightPoints [MaxPolygonVertices]Vec2
	rightCount := 0

	bestIndex := 0
	bestDistance := Cross(Sub(ps[bestIndex], p1), e)
	if bestDistance > 0.0 {
		rightPoints[rightCount] = ps[bestIndex]
		rightCount++
	}

	for i := 1; i < count; i++ {
		distance := Cross(Sub(ps[i], p1), e)
		if distance > bestDistance {
			bestIndex = i
			bestDistance = distance
		}

		if distance > 0.0 {
			rightPoints[rightCount] = ps[i]
			rightCount++
		}
	}

	if bestDistance < 2.0*LinearSlop {
		return hull
	}

	bestPoint := ps[bestIndex]

	// compute hull to the right of p1-bestPoint
	hull1 := recurseHull(p1, bestPoint, rightPoints[:rightCount])

	// compute hull to the right of bestPoint-p2
	hull2 := recurseHull(bestPoint, p2, rightPoints[:rightCount])

	// stitch together hulls
	for i := range hull1.Count {
		hull.Points[hull.Count] = hull1.Points[i]
		hull.Count++
	}

	hull.Points[hull.Count] = bestPoint
	hull.Count++

	for i := range hull2.Count {
		hull.Points[hull.Count] = hull2.Points[i]
		hull.Count++
	}

	assert(hull.Count < MaxPolygonVertices)

	return hull
}

// ComputeHull computes the convex hull of a set of points (upstream
// b2ComputeHull). It returns an empty hull if it fails. Some failure cases:
//   - all points very close together
//   - all points on a line
//   - less than 3 points
//   - more than MaxPolygonVertices points
//
// This welds close points and removes collinear points.
//
// Do not modify a hull once it has been computed.
//
// The quickhull algorithm:
//   - merges vertices based on LinearSlop
//   - removes collinear points using LinearSlop
//   - returns an empty hull if it fails
func ComputeHull(points []Vec2) Hull {
	var hull Hull
	hull.Count = 0

	count := len(points)
	if count < 3 || count > MaxPolygonVertices {
		// check your data
		return hull
	}

	count = minInt(count, MaxPolygonVertices)

	aabb := AABB{
		LowerBound: Vec2{math.MaxFloat64, math.MaxFloat64},
		UpperBound: Vec2{-math.MaxFloat64, -math.MaxFloat64},
	}

	// Perform aggressive point welding. First point always remains.
	// Also compute the bounding box for later.
	var ps [MaxPolygonVertices]Vec2
	n := 0
	linearSlop := LinearSlop
	tolSqr := 16.0 * linearSlop * linearSlop
	for i := range count {
		aabb.LowerBound = Min(aabb.LowerBound, points[i])
		aabb.UpperBound = Max(aabb.UpperBound, points[i])

		vi := points[i]

		unique := true
		for j := range i {
			vj := points[j]

			distSqr := DistanceSquared(vi, vj)
			if distSqr < tolSqr {
				unique = false
				break
			}
		}

		if unique {
			ps[n] = vi
			n++
		}
	}

	if n < 3 {
		// all points very close together, check your data and check your scale
		return hull
	}

	// Find an extreme point as the first point on the hull
	c := AABBCenter(aabb)
	f1 := 0
	dsq1 := DistanceSquared(c, ps[f1])
	for i := 1; i < n; i++ {
		dsq := DistanceSquared(c, ps[i])
		if dsq > dsq1 {
			f1 = i
			dsq1 = dsq
		}
	}

	// remove p1 from working set
	p1 := ps[f1]
	ps[f1] = ps[n-1]
	n--

	f2 := 0
	dsq2 := DistanceSquared(p1, ps[f2])
	for i := 1; i < n; i++ {
		dsq := DistanceSquared(p1, ps[i])
		if dsq > dsq2 {
			f2 = i
			dsq2 = dsq
		}
	}

	// remove p2 from working set
	p2 := ps[f2]
	ps[f2] = ps[n-1]
	n--

	// split the points into points that are left and right of the line p1-p2.
	var rightPoints [MaxPolygonVertices - 2]Vec2
	rightCount := 0

	var leftPoints [MaxPolygonVertices - 2]Vec2
	leftCount := 0

	e := Normalize(Sub(p2, p1))

	for i := range n {
		d := Cross(Sub(ps[i], p1), e)

		// slop used here to skip points that are very close to the line p1-p2
		if d >= 2.0*linearSlop {
			rightPoints[rightCount] = ps[i]
			rightCount++
		} else if d <= -2.0*linearSlop {
			leftPoints[leftCount] = ps[i]
			leftCount++
		}
	}

	// compute hulls on right and left
	hull1 := recurseHull(p1, p2, rightPoints[:rightCount])
	hull2 := recurseHull(p2, p1, leftPoints[:leftCount])

	if hull1.Count == 0 && hull2.Count == 0 {
		// all points collinear
		return hull
	}

	// stitch hulls together, preserving CCW winding order
	hull.Points[hull.Count] = p1
	hull.Count++

	for i := range hull1.Count {
		hull.Points[hull.Count] = hull1.Points[i]
		hull.Count++
	}

	hull.Points[hull.Count] = p2
	hull.Count++

	for i := range hull2.Count {
		hull.Points[hull.Count] = hull2.Points[i]
		hull.Count++
	}

	assert(hull.Count <= MaxPolygonVertices)

	// merge collinear
	searching := true
	for searching && hull.Count > 2 {
		searching = false

		for i := 0; i < hull.Count; i++ {
			i1 := i
			i2 := (i + 1) % hull.Count
			i3 := (i + 2) % hull.Count

			s1 := hull.Points[i1]
			s2 := hull.Points[i2]
			s3 := hull.Points[i3]

			// unit edge vector for s1-s3
			r := Normalize(Sub(s3, s1))

			distance := Cross(Sub(s2, s1), r)
			if distance <= 2.0*linearSlop {
				// remove midpoint from hull
				for j := i2; j < hull.Count-1; j++ {
					hull.Points[j] = hull.Points[j+1]
				}
				hull.Count--

				// continue searching for collinear points
				searching = true

				break
			}
		}
	}

	if hull.Count < 3 {
		// all points collinear, shouldn't be reached since this was validated above
		hull.Count = 0
	}

	return hull
}

// ValidateHull determines whether a hull is valid (upstream b2ValidateHull).
// It checks for:
//   - convexity
//   - collinear points
//
// This is expensive and should not be called at runtime.
func ValidateHull(hull *Hull) bool {
	if hull.Count < 3 || MaxPolygonVertices < hull.Count {
		return false
	}

	// test that every point is behind every edge
	for i := range hull.Count {
		// create an edge vector
		i1 := i
		i2 := 0
		if i < hull.Count-1 {
			i2 = i1 + 1
		}
		p := hull.Points[i1]
		e := Normalize(Sub(hull.Points[i2], p))

		for j := range hull.Count {
			// skip points that subtend the current edge
			if j == i1 || j == i2 {
				continue
			}

			distance := Cross(Sub(hull.Points[j], p), e)
			if distance >= 0.0 {
				return false
			}
		}
	}

	// test for collinear points
	linearSlop := LinearSlop
	for i := range hull.Count {
		i1 := i
		i2 := (i + 1) % hull.Count
		i3 := (i + 2) % hull.Count

		p1 := hull.Points[i1]
		p2 := hull.Points[i2]
		p3 := hull.Points[i3]

		e := Normalize(Sub(p3, p1))

		distance := Cross(Sub(p2, p1), e)
		if distance <= linearSlop {
			// p1-p2-p3 are collinear
			return false
		}
	}

	return true
}
