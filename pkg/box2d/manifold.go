// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/manifold.c.
// This port uses float64 where upstream uses float; all multiply-accumulate
// expressions are explicitly rounded (see math_fma.go).
//
// The ManifoldPoint and Manifold declarations at the top of this file come
// from include/box2d/collision.h upstream; they live here (rather than in
// collision.go) because this is the stage that ports them.
//
// The upstream file contains an `#if 1 / #else` block in b2CollidePolygons;
// only the enabled branch is ported. The disabled branch is dead code.

package box2d

import "math"

// ManifoldPoint is a contact point in a Manifold (upstream b2ManifoldPoint).
type ManifoldPoint struct {
	// ClipPoint is the location of the contact point in world space when first
	// clipped. Subject to precision loss at large coordinates. This point lags
	// behind when contact recycling is used. Should only be used for
	// debugging; use AnchorA and/or AnchorB for game logic.
	ClipPoint Vec2

	// AnchorA is the location of the contact point relative to shapeA's origin
	// in world space. When used internally to the Box2D solver, this is
	// relative to the body center of mass.
	AnchorA Vec2

	// AnchorB is the location of the contact point relative to shapeB's origin
	// in world space. When used internally to the Box2D solver, this is
	// relative to the body center of mass.
	AnchorB Vec2

	// Separation is the separation of the contact point, negative if
	// penetrating.
	Separation float64

	// BaseSeparation is the cached separation used for contact recycling.
	BaseSeparation float64

	// NormalImpulse is the impulse along the manifold normal vector.
	NormalImpulse float64

	// TangentImpulse is the friction impulse.
	TangentImpulse float64

	// TotalNormalImpulse is the total normal impulse applied across
	// sub-stepping and restitution. This is important to identify speculative
	// contact points that had an interaction in the time step. This includes
	// the warm starting impulse, the sub-step delta impulse, and the
	// restitution impulse.
	TotalNormalImpulse float64

	// NormalVelocity is the relative normal velocity pre-solve. Used for hit
	// events. If the normal impulse is zero then there was no hit. Negative
	// means shapes are approaching.
	NormalVelocity float64

	// ID uniquely identifies a contact point between two shapes.
	ID uint16

	// Persisted reports whether this contact point existed the previous step.
	Persisted bool
}

// Manifold describes the contact points between colliding shapes (upstream
// b2Manifold). Box2D uses speculative collision so some contact points may be
// separated.
type Manifold struct {
	// Normal is the unit normal vector in world space, points from shape A to
	// body B.
	Normal Vec2

	// RollingImpulse is the angular impulse applied for rolling resistance.
	// N * m * s = kg * m^2 / s.
	RollingImpulse float64

	// Points are the manifold points, up to two are possible in 2D.
	Points [2]ManifoldPoint

	// PointCount is the number of contact points, will be 0, 1, or 2.
	PointCount int
}

// makeID mirrors the upstream B2_MAKE_ID macro:
// ( (uint8_t)( A ) << 8 | (uint8_t)( B ) ).
func makeID(a, b int) uint16 {
	return uint16(uint8(a))<<8 | uint16(uint8(b))
}

// makeCapsule is the static b2MakeCapsule helper from manifold.c. It builds a
// 2-vertex rounded polygon representing a capsule.
func makeCapsule(p1, p2 Vec2, radius float64) Polygon {
	var shape Polygon
	shape.Vertices[0] = p1
	shape.Vertices[1] = p2
	shape.Centroid = Lerp(p1, p2, 0.5)

	d := Sub(p2, p1)
	assert(LengthSquared(d) > epsilon)
	axis := Normalize(d)
	normal := RightPerp(axis)

	shape.Normals[0] = normal
	shape.Normals[1] = Neg(normal)
	shape.Count = 2
	shape.Radius = radius

	return shape
}

// CollideCircles computes the contact manifold between two circles (upstream
// b2CollideCircles).
//
// point = qA * localAnchorA + pA
// localAnchorB = qBc * (point - pB)
// anchorB = point - pB = qA * localAnchorA + pA - pB
//
//	= anchorA + (pA - pB)
func CollideCircles(circleA *Circle, xfA Transform, circleB *Circle, xfB Transform) Manifold {
	var manifold Manifold

	xf := InvMulTransforms(xfA, xfB)

	pointA := circleA.Center
	pointB := TransformPoint(xf, circleB.Center)

	normal, distance := GetLengthAndNormalize(Sub(pointB, pointA))

	radiusA := circleA.Radius
	radiusB := circleB.Radius

	separation := distance - radiusA - radiusB
	if separation > SpeculativeDistance {
		return manifold
	}

	cA := MulAdd(pointA, radiusA, normal)
	cB := MulAdd(pointB, -radiusB, normal)
	contactPointA := Lerp(cA, cB, 0.5)

	manifold.Normal = RotateVector(xfA.Q, normal)
	mp := &manifold.Points[0]
	mp.AnchorA = RotateVector(xfA.Q, contactPointA)
	mp.AnchorB = Add(mp.AnchorA, Sub(xfA.P, xfB.P))
	mp.ClipPoint = Add(mp.AnchorA, xfA.P)
	mp.Separation = separation
	mp.ID = 0
	manifold.PointCount = 1
	return manifold
}

// CollideCapsuleAndCircle computes the collision manifold between a capsule
// and circle (upstream b2CollideCapsuleAndCircle).
func CollideCapsuleAndCircle(capsuleA *Capsule, xfA Transform, circleB *Circle, xfB Transform) Manifold {
	var manifold Manifold

	xf := InvMulTransforms(xfA, xfB)

	// Compute circle position in the frame of the capsule.
	pB := TransformPoint(xf, circleB.Center)

	// Compute closest point
	p1 := capsuleA.Center1
	p2 := capsuleA.Center2

	e := Sub(p2, p1)

	// dot(p - pA, e) = 0
	// dot(p - (p1 + s1 * e), e) = 0
	// s1 = dot(p - p1, e)
	// Upstream if/else-if/else chain; branch order preserved.
	var pA Vec2
	s1 := Dot(Sub(pB, p1), e)
	s2 := Dot(Sub(p2, pB), e)
	switch {
	case s1 < 0.0:
		// p1 region
		pA = p1
	case s2 < 0.0:
		// p2 region
		pA = p2
	default:
		// circle colliding with segment interior
		s := s1 / Dot(e, e)
		pA = MulAdd(p1, s, e)
	}

	normal, distance := GetLengthAndNormalize(Sub(pB, pA))

	radiusA := capsuleA.Radius
	radiusB := circleB.Radius
	separation := distance - radiusA - radiusB
	if separation > SpeculativeDistance {
		return manifold
	}

	cA := MulAdd(pA, radiusA, normal)
	cB := MulAdd(pB, -radiusB, normal)
	contactPointA := Lerp(cA, cB, 0.5)

	manifold.Normal = RotateVector(xfA.Q, normal)
	mp := &manifold.Points[0]
	mp.AnchorA = RotateVector(xfA.Q, contactPointA)
	mp.AnchorB = Add(mp.AnchorA, Sub(xfA.P, xfB.P))
	mp.ClipPoint = Add(xfA.P, mp.AnchorA)
	mp.Separation = separation
	mp.ID = 0
	manifold.PointCount = 1
	return manifold
}

// CollidePolygonAndCircle computes the collision manifold between a polygon
// and a circle (upstream b2CollidePolygonAndCircle).
func CollidePolygonAndCircle(polygonA *Polygon, xfA Transform, circleB *Circle, xfB Transform) Manifold {
	var manifold Manifold
	speculativeDistance := SpeculativeDistance

	xf := InvMulTransforms(xfA, xfB)

	// Compute circle position in the frame of the polygon.
	center := TransformPoint(xf, circleB.Center)
	radiusA := polygonA.Radius
	radiusB := circleB.Radius
	radius := radiusA + radiusB

	// Find the min separating edge.
	normalIndex := 0
	separation := -math.MaxFloat64
	vertexCount := polygonA.Count
	// Slicing to the live count lets the compiler drop the per-iteration
	// bounds checks in the loop below.
	vertices := polygonA.Vertices[:vertexCount]
	normals := polygonA.Normals[:vertexCount]

	for i := range vertexCount {
		s := Dot(normals[i], Sub(center, vertices[i]))
		if s > separation {
			separation = s
			normalIndex = i
		}
	}

	if separation > radius+speculativeDistance {
		return manifold
	}

	// Vertices of the reference edge.
	vertIndex1 := normalIndex
	vertIndex2 := 0
	if vertIndex1+1 < vertexCount {
		vertIndex2 = vertIndex1 + 1
	}
	v1 := vertices[vertIndex1]
	v2 := vertices[vertIndex2]

	// Compute barycentric coordinates
	u1 := Dot(Sub(center, v1), Sub(v2, v1))
	u2 := Dot(Sub(center, v2), Sub(v1, v2))

	// Upstream if/else-if/else chain; branch order preserved.
	switch {
	case u1 < 0.0 && separation > epsilon:
		// Circle center is closest to v1 and safely outside the polygon
		normal := Normalize(Sub(center, v1))
		separation = Dot(Sub(center, v1), normal)
		if separation > radius+speculativeDistance {
			return manifold
		}

		cA := MulAdd(v1, radiusA, normal)
		cB := MulSub(center, radiusB, normal)
		contactPointA := Lerp(cA, cB, 0.5)

		manifold.Normal = RotateVector(xfA.Q, normal)
		mp := &manifold.Points[0]
		mp.AnchorA = RotateVector(xfA.Q, contactPointA)
		mp.AnchorB = Add(mp.AnchorA, Sub(xfA.P, xfB.P))
		mp.ClipPoint = Add(xfA.P, mp.AnchorA)
		mp.Separation = Dot(Sub(cB, cA), normal)
		mp.ID = 0
		manifold.PointCount = 1
	case u2 < 0.0 && separation > epsilon:
		// Circle center is closest to v2 and safely outside the polygon
		normal := Normalize(Sub(center, v2))
		separation = Dot(Sub(center, v2), normal)
		if separation > radius+speculativeDistance {
			return manifold
		}

		cA := MulAdd(v2, radiusA, normal)
		cB := MulSub(center, radiusB, normal)
		contactPointA := Lerp(cA, cB, 0.5)

		manifold.Normal = RotateVector(xfA.Q, normal)
		mp := &manifold.Points[0]
		mp.AnchorA = RotateVector(xfA.Q, contactPointA)
		mp.AnchorB = Add(mp.AnchorA, Sub(xfA.P, xfB.P))
		mp.ClipPoint = Add(xfA.P, mp.AnchorA)
		mp.Separation = Dot(Sub(cB, cA), normal)
		mp.ID = 0
		manifold.PointCount = 1
	default:
		// Circle center is between v1 and v2. Center may be inside polygon
		normal := normals[normalIndex]
		manifold.Normal = RotateVector(xfA.Q, normal)

		// cA is the projection of the circle center onto to the reference edge
		cA := MulAdd(center, radiusA-Dot(Sub(center, v1), normal), normal)

		// cB is the deepest point on the circle with respect to the reference edge
		cB := MulSub(center, radiusB, normal)

		contactPointA := Lerp(cA, cB, 0.5)

		// The contact point is the midpoint in world space
		mp := &manifold.Points[0]
		mp.AnchorA = RotateVector(xfA.Q, contactPointA)
		mp.AnchorB = Add(mp.AnchorA, Sub(xfA.P, xfB.P))
		mp.ClipPoint = Add(xfA.P, mp.AnchorA)
		mp.Separation = separation - radius
		mp.ID = 0
		manifold.PointCount = 1
	}

	return manifold
}

// CollideCapsules computes the contact manifold between two capsules (upstream
// b2CollideCapsules).
//
// Follows Ericson 5.1.9 Closest Points of Two Line Segments. Adds some logic
// to support clipping to get two contact points.
func CollideCapsules(capsuleA *Capsule, xfA Transform, capsuleB *Capsule, xfB Transform) Manifold {
	origin := capsuleA.Center1

	// Shift polyA to origin
	// pw = q * pb + p
	// pw = q * (pbs + origin) + p
	// pw = q * pbs + (p + q * origin)
	sfA := Transform{P: Add(xfA.P, RotateVector(xfA.Q, origin)), Q: xfA.Q}
	xf := InvMulTransforms(sfA, xfB)

	p1 := Vec2Zero
	q1 := Sub(capsuleA.Center2, origin)

	p2 := TransformPoint(xf, capsuleB.Center1)
	q2 := TransformPoint(xf, capsuleB.Center2)

	d1 := Sub(q1, p1)
	d2 := Sub(q2, p2)

	dd1 := Dot(d1, d1)
	dd2 := Dot(d2, d2)

	const epsSqr = epsilon * epsilon
	assert(dd1 > epsSqr && dd2 > epsSqr)

	r := Sub(p1, p2)
	rd1 := Dot(r, d1)
	rd2 := Dot(r, d2)

	d12 := Dot(d1, d2)

	// upstream: dd1 * dd2 - d12 * d12
	denom := cross2(dd1, dd2, d12, d12)

	// Fraction on segment 1
	f1 := 0.0
	if denom != 0.0 {
		// not parallel
		// upstream: ( d12 * rd2 - rd1 * dd2 ) / denom
		f1 = clampFloat(cross2(d12, rd2, rd1, dd2)/denom, 0.0, 1.0)
	}

	// Compute point on segment 2 closest to p1 + f1 * d1
	// upstream: ( d12 * f1 + rd2 ) / dd2
	f2 := mulAdd(d12, f1, rd2) / dd2

	// Clamping of segment 2 requires a do over on segment 1
	if f2 < 0.0 {
		f2 = 0.0
		f1 = clampFloat(-rd1/dd1, 0.0, 1.0)
	} else if f2 > 1.0 {
		f2 = 1.0
		f1 = clampFloat((d12-rd1)/dd1, 0.0, 1.0)
	}

	closest1 := MulAdd(p1, f1, d1)
	closest2 := MulAdd(p2, f2, d2)
	distanceSquared := DistanceSquared(closest1, closest2)

	var manifold Manifold
	radiusA := capsuleA.Radius
	radiusB := capsuleB.Radius
	radius := radiusA + radiusB
	maxDistance := radius + SpeculativeDistance

	if distanceSquared > float64(maxDistance*maxDistance) {
		return manifold
	}

	distance := math.Sqrt(distanceSquared)

	u1, length1 := GetLengthAndNormalize(d1)
	u2, length2 := GetLengthAndNormalize(d2)

	// Does segment B project outside segment A?
	fp2 := Dot(Sub(p2, p1), u1)
	fq2 := Dot(Sub(q2, p1), u1)
	outsideA := (fp2 <= 0.0 && fq2 <= 0.0) || (fp2 >= length1 && fq2 >= length1)

	// Does segment A project outside segment B?
	fp1 := Dot(Sub(p1, p2), u2)
	fq1 := Dot(Sub(q1, p2), u2)
	outsideB := (fp1 <= 0.0 && fq1 <= 0.0) || (fp1 >= length2 && fq1 >= length2)

	if !outsideA && !outsideB {
		// attempt to clip
		// this may yield contact points with excessive separation
		// in that case the algorithm falls back to single point collision

		// find reference edge using SAT
		var normalA Vec2
		var separationA float64

		{
			normalA = LeftPerp(u1)
			ss1 := Dot(Sub(p2, p1), normalA)
			ss2 := Dot(Sub(q2, p1), normalA)
			s1p := ss2
			if ss1 < ss2 {
				s1p = ss1
			}
			s1n := -ss2
			if -ss1 < -ss2 {
				s1n = -ss1
			}

			if s1p > s1n {
				separationA = s1p
			} else {
				separationA = s1n
				normalA = Neg(normalA)
			}
		}

		var normalB Vec2
		var separationB float64
		{
			normalB = LeftPerp(u2)
			ss1 := Dot(Sub(p1, p2), normalB)
			ss2 := Dot(Sub(q1, p2), normalB)
			s1p := ss2
			if ss1 < ss2 {
				s1p = ss1
			}
			s1n := -ss2
			if -ss1 < -ss2 {
				s1n = -ss1
			}

			if s1p > s1n {
				separationB = s1p
			} else {
				separationB = s1n
				normalB = Neg(normalB)
			}
		}

		// biased to avoid feature flip-flop
		// todo more testing?
		slopBias := float64(0.1 * LinearSlop)
		if separationA+slopBias >= separationB {
			manifold.Normal = normalA

			cp := p2
			cq := q2

			// clip to p1
			if fp2 < 0.0 && fq2 > 0.0 {
				cp = Lerp(p2, q2, (0.0-fp2)/(fq2-fp2))
			} else if fq2 < 0.0 && fp2 > 0.0 {
				cq = Lerp(q2, p2, (0.0-fq2)/(fp2-fq2))
			}

			// clip to q1
			if fp2 > length1 && fq2 < length1 {
				cp = Lerp(p2, q2, (fp2-length1)/(fp2-fq2))
			} else if fq2 > length1 && fp2 < length1 {
				cq = Lerp(q2, p2, (fq2-length1)/(fq2-fp2))
			}

			sp := Dot(Sub(cp, p1), normalA)
			sq := Dot(Sub(cq, p1), normalA)

			if sp <= distance+LinearSlop || sq <= distance+LinearSlop {
				mp := &manifold.Points[0]
				mp.AnchorA = MulAdd(cp, 0.5*(radiusA-radiusB-sp), normalA)
				mp.Separation = sp - radius
				mp.ID = makeID(0, 0)

				mp = &manifold.Points[1]
				mp.AnchorA = MulAdd(cq, 0.5*(radiusA-radiusB-sq), normalA)
				mp.Separation = sq - radius
				mp.ID = makeID(0, 1)
				manifold.PointCount = 2
			}
		} else {
			// normal always points from A to B
			manifold.Normal = Neg(normalB)

			cp := p1
			cq := q1

			// clip to p2
			if fp1 < 0.0 && fq1 > 0.0 {
				cp = Lerp(p1, q1, (0.0-fp1)/(fq1-fp1))
			} else if fq1 < 0.0 && fp1 > 0.0 {
				cq = Lerp(q1, p1, (0.0-fq1)/(fp1-fq1))
			}

			// clip to q2
			if fp1 > length2 && fq1 < length2 {
				cp = Lerp(p1, q1, (fp1-length2)/(fp1-fq1))
			} else if fq1 > length2 && fp1 < length2 {
				cq = Lerp(q1, p1, (fq1-length2)/(fq1-fp1))
			}

			sp := Dot(Sub(cp, p2), normalB)
			sq := Dot(Sub(cq, p2), normalB)

			if sp <= distance+LinearSlop || sq <= distance+LinearSlop {
				mp := &manifold.Points[0]
				mp.AnchorA = MulAdd(cp, 0.5*(radiusB-radiusA-sp), normalB)
				mp.Separation = sp - radius
				mp.ID = makeID(0, 0)
				mp = &manifold.Points[1]
				mp.AnchorA = MulAdd(cq, 0.5*(radiusB-radiusA-sq), normalB)
				mp.Separation = sq - radius
				mp.ID = makeID(1, 0)
				manifold.PointCount = 2
			}
		}
	}

	if manifold.PointCount == 0 {
		// single point collision
		normal := Sub(closest2, closest1)
		if Dot(normal, normal) > epsSqr {
			normal = Normalize(normal)
		} else {
			normal = LeftPerp(u1)
		}

		c1 := MulAdd(closest1, radiusA, normal)
		c2 := MulAdd(closest2, -radiusB, normal)

		i1 := 1
		if f1 == 0.0 {
			i1 = 0
		}
		i2 := 1
		if f2 == 0.0 {
			i2 = 0
		}

		manifold.Normal = normal
		manifold.Points[0].AnchorA = Lerp(c1, c2, 0.5)
		manifold.Points[0].Separation = math.Sqrt(distanceSquared) - radius
		manifold.Points[0].ID = makeID(i1, i2)
		manifold.PointCount = 1
	}

	// Convert manifold to world space
	manifold.Normal = RotateVector(xfA.Q, manifold.Normal)
	for i := range manifold.PointCount {
		mp := &manifold.Points[i]

		// anchor points relative to shape origin in world space
		mp.AnchorA = RotateVector(xfA.Q, Add(mp.AnchorA, origin))
		mp.AnchorB = Add(mp.AnchorA, Sub(xfA.P, xfB.P))
		mp.ClipPoint = Add(xfA.P, mp.AnchorA)
	}

	return manifold
}

// CollideSegmentAndCapsule computes the contact manifold between a segment and
// a capsule (upstream b2CollideSegmentAndCapsule).
func CollideSegmentAndCapsule(segmentA *Segment, xfA Transform, capsuleB *Capsule, xfB Transform) Manifold {
	capsuleA := Capsule{Center1: segmentA.Point1, Center2: segmentA.Point2, Radius: 0.0}
	return CollideCapsules(&capsuleA, xfA, capsuleB, xfB)
}

// CollidePolygonAndCapsule computes the contact manifold between a polygon and
// a capsule (upstream b2CollidePolygonAndCapsule).
func CollidePolygonAndCapsule(polygonA *Polygon, xfA Transform, capsuleB *Capsule, xfB Transform) Manifold {
	polyB := makeCapsule(capsuleB.Center1, capsuleB.Center2, capsuleB.Radius)
	return CollidePolygons(polygonA, xfA, &polyB, xfB)
}

// clipPolygons is the polygon clipper used to compute contact points when
// there are potentially two contact points (upstream static b2ClipPolygons).
func clipPolygons(polyA, polyB *Polygon, edgeA, edgeB int, flip bool) Manifold {
	var manifold Manifold

	// reference polygon
	var poly1 *Polygon
	var i11, i12 int

	// incident polygon
	var poly2 *Polygon
	var i21, i22 int

	if flip {
		poly1 = polyB
		poly2 = polyA
		i11 = edgeB
		if edgeB+1 < polyB.Count {
			i12 = edgeB + 1
		} else {
			i12 = 0
		}
		i21 = edgeA
		if edgeA+1 < polyA.Count {
			i22 = edgeA + 1
		} else {
			i22 = 0
		}
	} else {
		poly1 = polyA
		poly2 = polyB
		i11 = edgeA
		if edgeA+1 < polyA.Count {
			i12 = edgeA + 1
		} else {
			i12 = 0
		}
		i21 = edgeB
		if edgeB+1 < polyB.Count {
			i22 = edgeB + 1
		} else {
			i22 = 0
		}
	}

	normal := poly1.Normals[i11]

	// Reference edge vertices
	v11 := poly1.Vertices[i11]
	v12 := poly1.Vertices[i12]

	// Incident edge vertices
	v21 := poly2.Vertices[i21]
	v22 := poly2.Vertices[i22]

	tangent := CrossSV(1.0, normal)

	lower1 := 0.0
	upper1 := Dot(Sub(v12, v11), tangent)

	// Incident edge points opposite of tangent due to CCW winding
	upper2 := Dot(Sub(v21, v11), tangent)
	lower2 := Dot(Sub(v22, v11), tangent)

	// Are the segments disjoint?
	if upper2 < lower1 || upper1 < lower2 {
		return manifold
	}

	var vLower Vec2
	if lower2 < lower1 && upper2-lower2 > epsilon {
		vLower = Lerp(v22, v21, (lower1-lower2)/(upper2-lower2))
	} else {
		vLower = v22
	}

	var vUpper Vec2
	if upper2 > upper1 && upper2-lower2 > epsilon {
		vUpper = Lerp(v22, v21, (upper1-lower2)/(upper2-lower2))
	} else {
		vUpper = v21
	}

	// todo vLower can be very close to vUpper, reduce to one point?

	separationLower := Dot(Sub(vLower, v11), normal)
	separationUpper := Dot(Sub(vUpper, v11), normal)

	r1 := poly1.Radius
	r2 := poly2.Radius

	// Put contact points at midpoint, accounting for radii
	vLower = MulAdd(vLower, 0.5*(r1-r2-separationLower), normal)
	vUpper = MulAdd(vUpper, 0.5*(r1-r2-separationUpper), normal)

	radius := r1 + r2

	if !flip {
		manifold.Normal = normal

		{
			cp := &manifold.Points[0]
			cp.AnchorA = vLower
			cp.Separation = separationLower - radius
			cp.ID = makeID(i11, i22)
			manifold.PointCount++
		}

		{
			cp := &manifold.Points[1]
			cp.AnchorA = vUpper
			cp.Separation = separationUpper - radius
			cp.ID = makeID(i12, i21)
			manifold.PointCount++
		}
	} else {
		manifold.Normal = Neg(normal)

		{
			cp := &manifold.Points[0]
			cp.AnchorA = vUpper
			cp.Separation = separationUpper - radius
			cp.ID = makeID(i21, i12)
			manifold.PointCount++
		}

		{
			cp := &manifold.Points[1]
			cp.AnchorA = vLower
			cp.Separation = separationLower - radius
			cp.ID = makeID(i22, i11)
			manifold.PointCount++
		}
	}

	return manifold
}

// findMaxSeparation finds the max separation between poly1 and poly2 using
// edge normals from poly1 (upstream static b2FindMaxSeparation). It returns
// the max separation and the edge index.
func findMaxSeparation(poly1, poly2 *Polygon) (float64, int) {
	count1 := poly1.Count
	count2 := poly2.Count
	// Slicing to the live counts lets the compiler drop the per-iteration
	// bounds checks in the nested loops below.
	n1s := poly1.Normals[:count1]
	v1s := poly1.Vertices[:count1]
	v2s := poly2.Vertices[:count2]

	bestIndex := 0
	maxSeparation := -math.MaxFloat64
	for i := range count1 {
		// Get poly1 normal in frame2.
		n := n1s[i]
		v1 := v1s[i]

		// Find the deepest point for normal i.
		si := math.MaxFloat64
		for j := range count2 {
			sij := Dot(n, Sub(v2s[j], v1))
			if sij < si {
				si = sij
			}
		}

		if si > maxSeparation {
			maxSeparation = si
			bestIndex = i
		}
	}

	return maxSeparation, bestIndex
}

// CollidePolygons computes the contact manifold between two polygons (upstream
// b2CollidePolygons).
//
// Due to speculation, every polygon is rounded.
// Algorithm:
//
//	compute edge separation using the separating axis test (SAT)
//	if (separation > speculation_distance)
//	  return
//	find reference and incident edge
//	if separation >= 0.1f * B2_LINEAR_SLOP
//	  compute closest points between reference and incident edge
//	  if vertices are closest
//	     single vertex-vertex contact
//	  else
//	     clip edges
//	  end
//	else
//	  clip edges
//	end
func CollidePolygons(polygonA *Polygon, xfA Transform, polygonB *Polygon, xfB Transform) Manifold {
	origin := polygonA.Vertices[0]
	linearSlop := LinearSlop
	speculativeDistance := SpeculativeDistance

	// Shift polyA to origin
	// pw = q * pb + p
	// pw = q * (pbs + origin) + p
	// pw = q * pbs + (p + q * origin)
	sfA := Transform{P: Add(xfA.P, RotateVector(xfA.Q, origin)), Q: xfA.Q}
	xf := InvMulTransforms(sfA, xfB)

	var localPolyA Polygon
	localPolyA.Count = polygonA.Count
	localPolyA.Radius = polygonA.Radius
	localPolyA.Vertices[0] = Vec2Zero
	localPolyA.Normals[0] = polygonA.Normals[0]
	for i := 1; i < localPolyA.Count; i++ {
		localPolyA.Vertices[i] = Sub(polygonA.Vertices[i], origin)
		localPolyA.Normals[i] = polygonA.Normals[i]
	}

	// Put polyB in polyA's frame to reduce round-off error
	var localPolyB Polygon
	localPolyB.Count = polygonB.Count
	localPolyB.Radius = polygonB.Radius
	for i := range localPolyB.Count {
		localPolyB.Vertices[i] = TransformPoint(xf, polygonB.Vertices[i])
		localPolyB.Normals[i] = RotateVector(xf.Q, polygonB.Normals[i])
	}

	separationA, edgeA := findMaxSeparation(&localPolyA, &localPolyB)
	separationB, edgeB := findMaxSeparation(&localPolyB, &localPolyA)

	radius := localPolyA.Radius + localPolyB.Radius

	if separationA > speculativeDistance+radius || separationB > speculativeDistance+radius {
		return Manifold{}
	}

	// Find incident edge
	var flip bool
	if separationA >= separationB {
		flip = false

		searchDirection := localPolyA.Normals[edgeA]

		// Find the incident edge on polyB
		count := localPolyB.Count
		normals := localPolyB.Normals[:count]
		edgeB = 0
		minDot := math.MaxFloat64
		for i := range count {
			dot := Dot(searchDirection, normals[i])
			if dot < minDot {
				minDot = dot
				edgeB = i
			}
		}
	} else {
		flip = true

		searchDirection := localPolyB.Normals[edgeB]

		// Find the incident edge on polyA
		count := localPolyA.Count
		normals := localPolyA.Normals[:count]
		edgeA = 0
		minDot := math.MaxFloat64
		for i := range count {
			dot := Dot(searchDirection, normals[i])
			if dot < minDot {
				minDot = dot
				edgeA = i
			}
		}
	}

	var manifold Manifold

	// Using slop here to ensure vertex-vertex normal vectors can be safely normalized
	// todo this means edge clipping needs to handle slightly non-overlapping edges.
	slopTenth := float64(0.1 * linearSlop)
	if separationA > slopTenth || separationB > slopTenth {
		// Edges are disjoint. Find closest points between reference edge and incident edge
		// Reference edge on polygon A
		i11 := edgeA
		i12 := 0
		if edgeA+1 < localPolyA.Count {
			i12 = edgeA + 1
		}
		i21 := edgeB
		i22 := 0
		if edgeB+1 < localPolyB.Count {
			i22 = edgeB + 1
		}

		v11 := localPolyA.Vertices[i11]
		v12 := localPolyA.Vertices[i12]
		v21 := localPolyB.Vertices[i21]
		v22 := localPolyB.Vertices[i22]

		result := SegmentDistance(v11, v12, v21, v22)
		assert(result.DistanceSquared > 0.0)
		distance := math.Sqrt(result.DistanceSquared)
		separation := distance - radius

		if distance-radius > speculativeDistance {
			// This can happen in the vertex-vertex case
			return manifold
		}

		// Attempt to clip edges
		manifold = clipPolygons(&localPolyA, &localPolyB, edgeA, edgeB, flip)

		minSeparation := math.MaxFloat64
		for i := range manifold.PointCount {
			minSeparation = minFloat(minSeparation, manifold.Points[i].Separation)
		}

		// Does vertex-vertex have substantially larger separation?
		// Upstream if/else-if chain; branch order preserved.
		if separation+slopTenth < minSeparation {
			switch {
			case result.Fraction1 == 0.0 && result.Fraction2 == 0.0:
				// v11 - v21
				normal := Sub(v21, v11)
				invDistance := 1.0 / distance
				normal.X *= invDistance
				normal.Y *= invDistance

				c1 := MulAdd(v11, localPolyA.Radius, normal)
				c2 := MulAdd(v21, -localPolyB.Radius, normal)

				manifold.Normal = normal
				manifold.Points[0].AnchorA = Lerp(c1, c2, 0.5)
				manifold.Points[0].Separation = distance - radius
				manifold.Points[0].ID = makeID(i11, i21)
				manifold.PointCount = 1
			case result.Fraction1 == 0.0 && result.Fraction2 == 1.0:
				// v11 - v22
				normal := Sub(v22, v11)
				invDistance := 1.0 / distance
				normal.X *= invDistance
				normal.Y *= invDistance

				c1 := MulAdd(v11, localPolyA.Radius, normal)
				c2 := MulAdd(v22, -localPolyB.Radius, normal)

				manifold.Normal = normal
				manifold.Points[0].AnchorA = Lerp(c1, c2, 0.5)
				manifold.Points[0].Separation = distance - radius
				manifold.Points[0].ID = makeID(i11, i22)
				manifold.PointCount = 1
			case result.Fraction1 == 1.0 && result.Fraction2 == 0.0:
				// v12 - v21
				normal := Sub(v21, v12)
				invDistance := 1.0 / distance
				normal.X *= invDistance
				normal.Y *= invDistance

				c1 := MulAdd(v12, localPolyA.Radius, normal)
				c2 := MulAdd(v21, -localPolyB.Radius, normal)

				manifold.Normal = normal
				manifold.Points[0].AnchorA = Lerp(c1, c2, 0.5)
				manifold.Points[0].Separation = distance - radius
				manifold.Points[0].ID = makeID(i12, i21)
				manifold.PointCount = 1
			case result.Fraction1 == 1.0 && result.Fraction2 == 1.0:
				// v12 - v22
				normal := Sub(v22, v12)
				invDistance := 1.0 / distance
				normal.X *= invDistance
				normal.Y *= invDistance

				c1 := MulAdd(v12, localPolyA.Radius, normal)
				c2 := MulAdd(v22, -localPolyB.Radius, normal)

				manifold.Normal = normal
				manifold.Points[0].AnchorA = Lerp(c1, c2, 0.5)
				manifold.Points[0].Separation = distance - radius
				manifold.Points[0].ID = makeID(i12, i22)
				manifold.PointCount = 1
			}
		}
	} else {
		// Polygons overlap
		manifold = clipPolygons(&localPolyA, &localPolyB, edgeA, edgeB, flip)
	}

	// Convert manifold to world space
	if manifold.PointCount > 0 {
		manifold.Normal = RotateVector(xfA.Q, manifold.Normal)
		for i := range manifold.PointCount {
			mp := &manifold.Points[i]

			// anchor points relative to shape origin in world space
			mp.AnchorA = RotateVector(xfA.Q, Add(mp.AnchorA, origin))
			mp.AnchorB = Add(mp.AnchorA, Sub(xfA.P, xfB.P))
			mp.ClipPoint = Add(xfA.P, mp.AnchorA)
		}
	}

	return manifold
}

// CollideSegmentAndCircle computes the contact manifold between a segment and
// a circle (upstream b2CollideSegmentAndCircle).
func CollideSegmentAndCircle(segmentA *Segment, xfA Transform, circleB *Circle, xfB Transform) Manifold {
	capsuleA := Capsule{Center1: segmentA.Point1, Center2: segmentA.Point2, Radius: 0.0}
	return CollideCapsuleAndCircle(&capsuleA, xfA, circleB, xfB)
}

// CollideSegmentAndPolygon computes the contact manifold between a segment and
// a rounded polygon (upstream b2CollideSegmentAndPolygon).
func CollideSegmentAndPolygon(segmentA *Segment, xfA Transform, polygonB *Polygon, xfB Transform) Manifold {
	polygonA := makeCapsule(segmentA.Point1, segmentA.Point2, 0.0)
	return CollidePolygons(&polygonA, xfA, polygonB, xfB)
}

// CollideChainSegmentAndCircle computes the contact manifold between a chain
// segment and a circle (upstream b2CollideChainSegmentAndCircle).
func CollideChainSegmentAndCircle(segmentA *ChainSegment, xfA Transform, circleB *Circle, xfB Transform) Manifold {
	var manifold Manifold

	xf := InvMulTransforms(xfA, xfB)

	// Compute circle in frame of segment
	pB := TransformPoint(xf, circleB.Center)

	p1 := segmentA.Segment.Point1
	p2 := segmentA.Segment.Point2
	e := Sub(p2, p1)

	// Normal points to the right
	offset := Dot(RightPerp(e), Sub(pB, p1))
	if offset < 0.0 {
		// collision is one-sided
		return manifold
	}

	// Barycentric coordinates
	u := Dot(e, Sub(p2, pB))
	v := Dot(e, Sub(pB, p1))

	var pA Vec2

	// Upstream if/else-if/else chain; branch order preserved.
	switch {
	case v <= 0.0:
		// Behind point1?
		// Is pB in the Voronoi region of the previous edge?
		prevEdge := Sub(p1, segmentA.Ghost1)
		uPrev := Dot(prevEdge, Sub(pB, p1))
		if uPrev <= 0.0 {
			return manifold
		}

		pA = p1
	case u <= 0.0:
		// Ahead of point2?
		nextEdge := Sub(segmentA.Ghost2, p2)
		vNext := Dot(nextEdge, Sub(pB, p2))

		// Is pB in the Voronoi region of the next edge?
		if vNext > 0.0 {
			return manifold
		}

		pA = p2
	default:
		ee := Dot(e, e)
		// upstream: { u * p1.x + v * p2.x, u * p1.y + v * p2.y }
		pA = Vec2{dot2(u, p1.X, v, p2.X), dot2(u, p1.Y, v, p2.Y)}
		if ee > 0.0 {
			pA = MulSV(1.0/ee, pA)
		} else {
			pA = p1
		}
	}

	normal, distance := GetLengthAndNormalize(Sub(pB, pA))

	radius := circleB.Radius
	separation := distance - radius
	if separation > SpeculativeDistance {
		return manifold
	}

	cA := pA
	cB := MulAdd(pB, -radius, normal)
	contactPointA := Lerp(cA, cB, 0.5)

	manifold.Normal = RotateVector(xfA.Q, normal)

	mp := &manifold.Points[0]
	mp.AnchorA = RotateVector(xfA.Q, contactPointA)
	mp.AnchorB = Add(mp.AnchorA, Sub(xfA.P, xfB.P))
	mp.ClipPoint = Add(xfA.P, mp.AnchorA)
	mp.Separation = separation
	mp.ID = 0
	manifold.PointCount = 1
	return manifold
}

// CollideChainSegmentAndCapsule computes the contact manifold between a chain
// segment and a capsule (upstream b2CollideChainSegmentAndCapsule).
func CollideChainSegmentAndCapsule(segmentA *ChainSegment, xfA Transform, capsuleB *Capsule, xfB Transform, cache *SimplexCache) Manifold {
	polyB := makeCapsule(capsuleB.Center1, capsuleB.Center2, capsuleB.Radius)
	return CollideChainSegmentAndPolygon(segmentA, xfA, &polyB, xfB, cache)
}

// clipSegments is upstream static b2ClipSegments.
func clipSegments(a1, a2, b1, b2 Vec2, normal Vec2, ra, rb float64, id1, id2 uint16) Manifold {
	var manifold Manifold

	tangent := LeftPerp(normal)

	// Barycentric coordinates of each point relative to a1 along tangent
	lower1 := 0.0
	upper1 := Dot(Sub(a2, a1), tangent)

	// Incident edge points opposite of tangent due to CCW winding
	upper2 := Dot(Sub(b1, a1), tangent)
	lower2 := Dot(Sub(b2, a1), tangent)

	// Do segments overlap?
	if upper2 < lower1 || upper1 < lower2 {
		return manifold
	}

	var vLower Vec2
	if lower2 < lower1 && upper2-lower2 > epsilon {
		vLower = Lerp(b2, b1, (lower1-lower2)/(upper2-lower2))
	} else {
		vLower = b2
	}

	var vUpper Vec2
	if upper2 > upper1 && upper2-lower2 > epsilon {
		vUpper = Lerp(b2, b1, (upper1-lower2)/(upper2-lower2))
	} else {
		vUpper = b1
	}

	// todo vLower can be very close to vUpper, reduce to one point?

	separationLower := Dot(Sub(vLower, a1), normal)
	separationUpper := Dot(Sub(vUpper, a1), normal)

	// Put contact points at midpoint, accounting for radii
	vLower = MulAdd(vLower, 0.5*(ra-rb-separationLower), normal)
	vUpper = MulAdd(vUpper, 0.5*(ra-rb-separationUpper), normal)

	radius := ra + rb

	manifold.Normal = normal
	{
		cp := &manifold.Points[0]
		cp.AnchorA = vLower
		cp.Separation = separationLower - radius
		cp.ID = id1
	}

	{
		cp := &manifold.Points[1]
		cp.AnchorA = vUpper
		cp.Separation = separationUpper - radius
		cp.ID = id2
	}

	manifold.PointCount = 2

	return manifold
}

// normalType mirrors the upstream enum b2NormalType.
type normalType int

const (
	// normalSkip means the normal points in a direction that is non-smooth
	// relative to a convex vertex and should be skipped (upstream
	// b2_normalSkip).
	normalSkip normalType = iota

	// normalAdmit means the normal points in a direction that is smooth
	// relative to a convex vertex and should be used for collision (upstream
	// b2_normalAdmit).
	normalAdmit

	// normalSnap means the normal is in a region of a concave vertex and
	// should be snapped to the segment normal (upstream b2_normalSnap).
	normalSnap
)

// chainSegmentParams mirrors upstream struct b2ChainSegmentParams.
type chainSegmentParams struct {
	edge1   Vec2
	normal0 Vec2
	normal2 Vec2
	convex1 bool
	convex2 bool
}

// classifyNormal evaluates the Gauss map (upstream static b2ClassifyNormal).
// See https://box2d.org/posts/2020/06/ghost-collisions/
func classifyNormal(params chainSegmentParams, normal Vec2) normalType {
	const sinTol = 0.01

	if Dot(normal, params.edge1) <= 0.0 {
		// Normal points towards the segment tail
		if params.convex1 {
			if Cross(normal, params.normal0) > sinTol {
				return normalSkip
			}

			return normalAdmit
		}

		return normalSnap
	}

	// Normal points towards segment head
	if params.convex2 {
		if Cross(params.normal2, normal) > sinTol {
			return normalSkip
		}

		return normalAdmit
	}

	return normalSnap
}

// CollideChainSegmentAndPolygon computes the contact manifold between a chain
// segment and a rounded polygon (upstream b2CollideChainSegmentAndPolygon).
func CollideChainSegmentAndPolygon(segmentA *ChainSegment, xfA Transform, polygonB *Polygon, xfB Transform, cache *SimplexCache) Manifold {
	// Public API precondition: the SAT loops below walk local
	// [MaxPolygonVertices]Vec2 arrays with polygonB.Count, so a hand-built
	// Polygon with an out-of-range Count must fail here rather than inside the
	// separation search. CollideChainSegmentAndCapsule reaches this function
	// with a 2-vertex polygon from makeCapsule, hence the count-1 lower bound.
	requireValidPolygonCount(polygonB)

	var manifold Manifold

	xf := InvMulTransforms(xfA, xfB)

	centroidB := TransformPoint(xf, polygonB.Centroid)
	radiusB := polygonB.Radius

	p1 := segmentA.Segment.Point1
	p2 := segmentA.Segment.Point2

	edge1 := Normalize(Sub(p2, p1))

	var smoothParams chainSegmentParams
	smoothParams.edge1 = edge1

	const convexTol = 0.01
	edge0 := Normalize(Sub(p1, segmentA.Ghost1))
	smoothParams.normal0 = RightPerp(edge0)
	smoothParams.convex1 = Cross(edge0, edge1) >= convexTol

	edge2 := Normalize(Sub(segmentA.Ghost2, p2))
	smoothParams.normal2 = RightPerp(edge2)
	smoothParams.convex2 = Cross(edge1, edge2) >= convexTol

	// Normal points to the right
	normal1 := RightPerp(edge1)
	behind1 := Dot(normal1, Sub(centroidB, p1)) < 0.0
	behind0 := true
	behind2 := true
	if smoothParams.convex1 {
		behind0 = Dot(smoothParams.normal0, Sub(centroidB, p1)) < 0.0
	}

	if smoothParams.convex2 {
		behind2 = Dot(smoothParams.normal2, Sub(centroidB, p2)) < 0.0
	}

	if behind1 && behind0 && behind2 {
		// one-sided collision
		return manifold
	}

	// Get polygonB in frameA
	count := polygonB.Count
	var vertices [MaxPolygonVertices]Vec2
	var normals [MaxPolygonVertices]Vec2
	for i := range count {
		//nolint:gosec // G602: count is polygonB.Count, validated to 1..MaxPolygonVertices by requireValidPolygonCount at the top of CollideChainSegmentAndPolygon; vertices is [MaxPolygonVertices]Vec2.
		vertices[i] = TransformPoint(xf, polygonB.Vertices[i])
		//nolint:gosec // G602: same bound as the line above; normals is [MaxPolygonVertices]Vec2.
		normals[i] = RotateVector(xf.Q, polygonB.Normals[i])
	}

	// Distance doesn't work correctly with partial polygons
	var input DistanceInput
	input.ProxyA = MakeProxy([]Vec2{segmentA.Segment.Point1, segmentA.Segment.Point2}, 2, 0.0)
	input.ProxyB = MakeProxy(vertices[:], count, 0.0)
	input.TransformA = TransformIdentity
	input.TransformB = TransformIdentity
	input.UseRadii = false

	output := ShapeDistance(&input, cache, nil)

	if output.Distance > radiusB+SpeculativeDistance {
		return manifold
	}

	// Snap concave normals for partial polygon
	n0 := normal1
	if smoothParams.convex1 {
		n0 = smoothParams.normal0
	}
	n2 := normal1
	if smoothParams.convex2 {
		n2 = smoothParams.normal2
	}

	// Index of incident vertex on polygon
	incidentIndex := -1
	incidentNormal := -1

	slopTenth := float64(0.1 * LinearSlop)
	if !behind1 && output.Distance > slopTenth {
		// The closest features may be two vertices or an edge and a vertex even when there should
		// be face contact

		if cache.Count == 1 {
			// vertex-vertex collision
			pA := output.PointA
			pB := output.PointB

			normal := Normalize(Sub(pB, pA))

			typ := classifyNormal(smoothParams, normal)
			if typ == normalSkip {
				return manifold
			}

			if typ == normalAdmit {
				manifold.Normal = RotateVector(xfA.Q, normal)
				cp := &manifold.Points[0]
				cp.AnchorA = RotateVector(xfA.Q, pA)
				cp.AnchorB = Add(cp.AnchorA, Sub(xfA.P, xfB.P))
				cp.ClipPoint = Add(xfA.P, cp.AnchorA)
				cp.Separation = output.Distance - radiusB
				cp.ID = makeID(int(cache.IndexA[0]), int(cache.IndexB[0]))
				manifold.PointCount = 1
				return manifold
			}

			// fall through normalSnap
			incidentIndex = int(cache.IndexB[0])
		} else {
			// vertex-edge collision
			assert(cache.Count == 2)

			ia1 := int(cache.IndexA[0])
			ia2 := int(cache.IndexA[1])
			ib1 := int(cache.IndexB[0])
			ib2 := int(cache.IndexB[1])

			if ia1 == ia2 {
				// 1 point on A, expect 2 points on B
				assert(ib1 != ib2)

				// Find polygon normal most aligned with vector between closest points.
				// This effectively sorts ib1 and ib2
				normalB := Sub(output.PointA, output.PointB)
				dot1 := Dot(normalB, normals[ib1])
				dot2 := Dot(normalB, normals[ib2])
				ib := ib2
				if dot1 > dot2 {
					ib = ib1
				}

				// Use accurate normal
				normalB = normals[ib]

				typ := classifyNormal(smoothParams, Neg(normalB))
				if typ == normalSkip {
					return manifold
				}

				if typ == normalAdmit {
					// Get polygon edge associated with normal
					ib1 = ib
					if ib < count-1 {
						ib2 = ib + 1
					} else {
						ib2 = 0
					}

					b1 := vertices[ib1]
					b2 := vertices[ib2]

					// Find incident segment vertex
					dot1 = Dot(normalB, Sub(p1, b1))
					dot2 = Dot(normalB, Sub(p2, b1))

					if dot1 < dot2 {
						if Dot(n0, normalB) < Dot(normal1, normalB) {
							// Neighbor is incident
							return manifold
						}
					} else {
						if Dot(n2, normalB) < Dot(normal1, normalB) {
							// Neighbor is incident
							return manifold
						}
					}

					manifold = clipSegments(b1, b2, p1, p2, normalB, radiusB, 0.0, makeID(ib1, 1), makeID(ib2, 0))

					assert(manifold.PointCount == 0 || manifold.PointCount == 2)
					if manifold.PointCount == 2 {
						manifold.Normal = RotateVector(xfA.Q, Neg(normalB))
						manifold.Points[0].AnchorA = RotateVector(xfA.Q, manifold.Points[0].AnchorA)
						manifold.Points[1].AnchorA = RotateVector(xfA.Q, manifold.Points[1].AnchorA)
						pAB := Sub(xfA.P, xfB.P)
						manifold.Points[0].AnchorB = Add(manifold.Points[0].AnchorA, pAB)
						manifold.Points[1].AnchorB = Add(manifold.Points[1].AnchorA, pAB)
						manifold.Points[0].ClipPoint = Add(xfA.P, manifold.Points[0].AnchorA)
						manifold.Points[1].ClipPoint = Add(xfA.P, manifold.Points[1].AnchorA)
					}
					return manifold
				}

				// fall through normalSnap
				incidentNormal = ib
			} else {
				// Get index of incident polygonB vertex
				dot1 := Dot(normal1, Sub(vertices[ib1], p1))
				dot2 := Dot(normal1, Sub(vertices[ib2], p2))
				incidentIndex = ib2
				if dot1 < dot2 {
					incidentIndex = ib1
				}
			}
		}
	} else {
		// SAT edge normal
		edgeSeparation := math.MaxFloat64

		for i := range count {
			//nolint:gosec // G602: count is polygonB.Count, validated to 1..MaxPolygonVertices by requireValidPolygonCount at the top of CollideChainSegmentAndPolygon; vertices is [MaxPolygonVertices]Vec2.
			s := Dot(normal1, Sub(vertices[i], p1))
			if s < edgeSeparation {
				edgeSeparation = s
				incidentIndex = i
			}
		}

		// Check convex neighbor for edge separation
		if smoothParams.convex1 {
			s0 := math.MaxFloat64

			for i := range count {
				//nolint:gosec // G602: count is polygonB.Count, validated to 1..MaxPolygonVertices by requireValidPolygonCount at the top of CollideChainSegmentAndPolygon; vertices is [MaxPolygonVertices]Vec2.
				s := Dot(smoothParams.normal0, Sub(vertices[i], p1))
				if s < s0 {
					s0 = s
				}
			}

			if s0 > edgeSeparation {
				edgeSeparation = s0

				// Indicate neighbor owns edge separation
				incidentIndex = -1
			}
		}

		// Check convex neighbor for edge separation
		if smoothParams.convex2 {
			s2 := math.MaxFloat64

			for i := range count {
				//nolint:gosec // G602: count is polygonB.Count, validated to 1..MaxPolygonVertices by requireValidPolygonCount at the top of CollideChainSegmentAndPolygon; vertices is [MaxPolygonVertices]Vec2.
				s := Dot(smoothParams.normal2, Sub(vertices[i], p2))
				if s < s2 {
					s2 = s
				}
			}

			if s2 > edgeSeparation {
				edgeSeparation = s2

				// Indicate neighbor owns edge separation
				incidentIndex = -1
			}
		}

		// SAT polygon normals
		polygonSeparation := -math.MaxFloat64
		referenceIndex := -1

		for i := range count {
			//nolint:gosec // G602: count is polygonB.Count, validated to 1..MaxPolygonVertices by requireValidPolygonCount at the top of CollideChainSegmentAndPolygon; normals is [MaxPolygonVertices]Vec2.
			n := normals[i]

			typ := classifyNormal(smoothParams, Neg(n))
			if typ != normalAdmit {
				continue
			}

			// Check the infinite sides of the partial polygon
			// if ((smoothParams.convex1 && b2Cross(n0, n) > 0.0f) || (smoothParams.convex2 && b2Cross(n, n2) > 0.0f))
			//{
			//	continue;
			//}

			p := vertices[i]
			s := minFloat(Dot(n, Sub(p2, p)), Dot(n, Sub(p1, p)))

			if s > polygonSeparation {
				polygonSeparation = s
				referenceIndex = i
			}
		}

		if polygonSeparation > edgeSeparation {
			ia1 := referenceIndex
			ia2 := 0
			if ia1 < count-1 {
				ia2 = ia1 + 1
			}
			a1 := vertices[ia1]
			a2 := vertices[ia2]

			n := normals[ia1]

			dot1 := Dot(n, Sub(p1, a1))
			dot2 := Dot(n, Sub(p2, a1))

			if dot1 < dot2 {
				if Dot(n0, n) < Dot(normal1, n) {
					// Neighbor is incident
					return manifold
				}
			} else {
				if Dot(n2, n) < Dot(normal1, n) {
					// Neighbor is incident
					return manifold
				}
			}

			manifold = clipSegments(a1, a2, p1, p2, normals[ia1], radiusB, 0.0, makeID(ia1, 1), makeID(ia2, 0))

			assert(manifold.PointCount == 0 || manifold.PointCount == 2)
			if manifold.PointCount == 2 {
				manifold.Normal = RotateVector(xfA.Q, Neg(normals[ia1]))
				manifold.Points[0].AnchorA = RotateVector(xfA.Q, manifold.Points[0].AnchorA)
				manifold.Points[1].AnchorA = RotateVector(xfA.Q, manifold.Points[1].AnchorA)
				pAB := Sub(xfA.P, xfB.P)
				manifold.Points[0].AnchorB = Add(manifold.Points[0].AnchorA, pAB)
				manifold.Points[1].AnchorB = Add(manifold.Points[1].AnchorA, pAB)
				manifold.Points[0].ClipPoint = Add(xfA.P, manifold.Points[0].AnchorA)
				manifold.Points[1].ClipPoint = Add(xfA.P, manifold.Points[1].AnchorA)
			}

			return manifold
		}

		if incidentIndex == -1 {
			// neighboring segment is the separating axis
			return manifold
		}

		// fall through segment normal axis
	}

	assert(incidentNormal != -1 || incidentIndex != -1)

	// Segment normal

	// Find incident polygon normal: normal adjacent to deepest vertex that is most anti-parallel to segment normal
	var b1, b2 Vec2
	var ib1, ib2 int

	if incidentNormal != -1 {
		ib1 = incidentNormal
		if ib1 < count-1 {
			ib2 = ib1 + 1
		} else {
			ib2 = 0
		}
		b1 = vertices[ib1]
		b2 = vertices[ib2]
	} else {
		i2 := incidentIndex
		i1 := count - 1
		if i2 > 0 {
			i1 = i2 - 1
		}
		d1 := Dot(normal1, normals[i1])
		d2 := Dot(normal1, normals[i2])
		if d1 < d2 {
			ib1, ib2 = i1, i2
			b1 = vertices[ib1]
			b2 = vertices[ib2]
		} else {
			ib1 = i2
			if i2 < count-1 {
				ib2 = i2 + 1
			} else {
				ib2 = 0
			}
			b1 = vertices[ib1]
			b2 = vertices[ib2]
		}
	}

	manifold = clipSegments(p1, p2, b1, b2, normal1, 0.0, radiusB, makeID(0, ib2), makeID(1, ib1))

	assert(manifold.PointCount == 0 || manifold.PointCount == 2)
	if manifold.PointCount == 2 {
		// There may be no points c
		manifold.Normal = RotateVector(xfA.Q, manifold.Normal)
		manifold.Points[0].AnchorA = RotateVector(xfA.Q, manifold.Points[0].AnchorA)
		manifold.Points[1].AnchorA = RotateVector(xfA.Q, manifold.Points[1].AnchorA)
		pAB := Sub(xfA.P, xfB.P)
		manifold.Points[0].AnchorB = Add(manifold.Points[0].AnchorA, pAB)
		manifold.Points[1].AnchorB = Add(manifold.Points[1].AnchorA, pAB)
		manifold.Points[0].ClipPoint = Add(xfA.P, manifold.Points[0].AnchorA)
		manifold.Points[1].ClipPoint = Add(xfA.P, manifold.Points[1].AnchorA)
	}

	return manifold
}
