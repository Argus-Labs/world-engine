// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/geometry.c.
//
// Functions whose upstream bodies call into src/distance.c (b2ShapeCast,
// b2ShapeDistance, b2MakeProxy) were deferred to stage E3 and are now
// complete (see distance.go).

package box2d

import "math"

// IsValidRay validates ray cast input data (NaN, etc.) (upstream b2IsValidRay).
func IsValidRay(input *RayCastInput) bool {
	isValid := IsValidVec2(input.Origin) && IsValidVec2(input.Translation) &&
		IsValidFloat(input.MaxFraction) && 0.0 <= input.MaxFraction && input.MaxFraction < Huge
	return isValid
}

// computePolygonCentroid is upstream b2ComputePolygonCentroid.
func computePolygonCentroid(vertices []Vec2) Vec2 {
	center := Vec2{0.0, 0.0}
	area := 0.0

	count := len(vertices)

	// Get a reference point for forming triangles.
	// Use the first vertex to reduce round-off errors.
	origin := vertices[0]

	const inv3 float64 = 1.0 / 3.0

	for i := 1; i < count-1; i++ {
		// Triangle edges
		e1 := Sub(vertices[i], origin)
		e2 := Sub(vertices[i+1], origin)
		a := float64(0.5 * Cross(e1, e2))

		// Area weighted centroid
		center = MulAdd(center, a*inv3, Add(e1, e2))
		area += a
	}

	assert(area > epsilon)
	invArea := 1.0 / area
	center.X *= invArea
	center.Y *= invArea

	// Restore offset
	center = Add(origin, center)

	return center
}

// Requirement strings for the Polygon.Count preconditions below. They are
// shared so the panic text is identical wherever the precondition is enforced.
const (
	polygonCountRequirement = "must be between 1 and MaxPolygonVertices; " +
		"build polygons with MakePolygon, MakeBox or a related constructor"
	polygonShapeCountRequirement = "must be between 3 and MaxPolygonVertices; " +
		"build polygons with MakePolygon, MakeBox or a related constructor"
)

// requireValidPolygonCount enforces the public API precondition that a Polygon
// entering the collision or mass-properties code has a vertex count its
// fixed-size Vertices and Normals arrays ([MaxPolygonVertices]Vec2) can hold.
//
// Polygon is exported with exported fields, so a caller can hand-build one by
// literal instead of going through ComputeHull plus MakePolygon (or MakeBox and
// friends). Without this check a Count outside the array bounds surfaces as an
// opaque index-out-of-range panic deep inside the collision routines, far from
// the mistake. Like the other require* helpers in core.go it is always enabled,
// independent of debugAsserts, and it does not touch valid inputs.
//
// The lower bound is 1, not 3: the port itself builds degenerate 1- and
// 2-vertex polygons for circles and capsules (see makeCapsule) and feeds them
// to these same routines. Shape creation uses the stricter
// requireValidPolygonShapeCount.
func requireValidPolygonCount(polygon *Polygon) {
	requireValidDefField(1 <= polygon.Count && polygon.Count <= MaxPolygonVertices,
		"Polygon", "Count", polygonCountRequirement)
}

// requireValidPolygonShapeCount is requireValidPolygonCount for the shape
// creation entry points, where only a real convex polygon is meaningful, so the
// count must be at least 3. Catching a bad Count here keeps it from being
// stored in the world and then panicking during a later World.Step, arbitrarily
// far from the call that introduced it.
func requireValidPolygonShapeCount(polygon *Polygon) {
	requireValidDefField(3 <= polygon.Count && polygon.Count <= MaxPolygonVertices,
		"Polygon", "Count", polygonShapeCountRequirement)
}

// MakePolygon makes a convex polygon from a convex hull (upstream
// b2MakePolygon). This will assert if the hull is not valid.
//
// Do not manually fill in the hull data, it must come directly from ComputeHull.
func MakePolygon(hull *Hull, radius float64) Polygon {
	if debugAsserts {
		assert(ValidateHull(hull))
	}

	if hull.Count < 3 {
		// Handle a bad hull when assertions are disabled
		return MakeSquare(0.5)
	}

	var shape Polygon
	shape.Count = hull.Count
	shape.Radius = radius

	// Copy vertices
	for i := range shape.Count {
		shape.Vertices[i] = hull.Points[i]
	}

	// Compute normals. Ensure the edges have non-zero length.
	for i := range shape.Count {
		i1 := i
		i2 := 0
		if i+1 < shape.Count {
			i2 = i + 1
		}
		edge := Sub(shape.Vertices[i2], shape.Vertices[i1])
		assert(Dot(edge, edge) > epsilon*epsilon)
		shape.Normals[i] = Normalize(CrossVS(edge, 1.0))
	}

	shape.Centroid = computePolygonCentroid(shape.Vertices[:shape.Count])

	return shape
}

// MakeOffsetPolygon makes an offset convex polygon from a convex hull
// (upstream b2MakeOffsetPolygon). This will assert if the hull is not valid.
//
// Do not manually fill in the hull data, it must come directly from ComputeHull.
func MakeOffsetPolygon(hull *Hull, position Vec2, rotation Rot) Polygon {
	return MakeOffsetRoundedPolygon(hull, position, rotation, 0.0)
}

// MakeOffsetRoundedPolygon makes an offset rounded convex polygon from a
// convex hull (upstream b2MakeOffsetRoundedPolygon). This will assert if the
// hull is not valid.
//
// Do not manually fill in the hull data, it must come directly from ComputeHull.
func MakeOffsetRoundedPolygon(hull *Hull, position Vec2, rotation Rot, radius float64) Polygon {
	if debugAsserts {
		assert(ValidateHull(hull))
	}

	if hull.Count < 3 {
		// Handle a bad hull when assertions are disabled
		return MakeSquare(0.5)
	}

	transform := Transform{P: position, Q: rotation}

	var shape Polygon
	shape.Count = hull.Count
	shape.Radius = radius

	// Copy vertices
	for i := range shape.Count {
		shape.Vertices[i] = TransformPoint(transform, hull.Points[i])
	}

	// Compute normals. Ensure the edges have non-zero length.
	for i := range shape.Count {
		i1 := i
		i2 := 0
		if i+1 < shape.Count {
			i2 = i + 1
		}
		edge := Sub(shape.Vertices[i2], shape.Vertices[i1])
		assert(Dot(edge, edge) > epsilon*epsilon)
		shape.Normals[i] = Normalize(CrossVS(edge, 1.0))
	}

	shape.Centroid = computePolygonCentroid(shape.Vertices[:shape.Count])

	return shape
}

// MakeSquare makes a square polygon, bypassing the need for a convex hull
// (upstream b2MakeSquare). halfWidth is the half-width.
func MakeSquare(halfWidth float64) Polygon {
	return MakeBox(halfWidth, halfWidth)
}

// MakeBox makes a box (rectangle) polygon, bypassing the need for a convex
// hull (upstream b2MakeBox). halfWidth is the half-width (x-axis) and
// halfHeight is the half-height (y-axis).
func MakeBox(halfWidth, halfHeight float64) Polygon {
	assert(IsValidFloat(halfWidth) && halfWidth > 0.0)
	assert(IsValidFloat(halfHeight) && halfHeight > 0.0)

	var shape Polygon
	shape.Count = 4
	shape.Vertices[0] = Vec2{-halfWidth, -halfHeight}
	shape.Vertices[1] = Vec2{halfWidth, -halfHeight}
	shape.Vertices[2] = Vec2{halfWidth, halfHeight}
	shape.Vertices[3] = Vec2{-halfWidth, halfHeight}
	shape.Normals[0] = Vec2{0.0, -1.0}
	shape.Normals[1] = Vec2{1.0, 0.0}
	shape.Normals[2] = Vec2{0.0, 1.0}
	shape.Normals[3] = Vec2{-1.0, 0.0}
	shape.Radius = 0.0
	shape.Centroid = Vec2Zero
	return shape
}

// MakeRoundedBox makes a rounded box, bypassing the need for a convex hull
// (upstream b2MakeRoundedBox). radius is the radius of the rounded extension.
func MakeRoundedBox(halfWidth, halfHeight, radius float64) Polygon {
	assert(IsValidFloat(radius) && radius >= 0.0)
	shape := MakeBox(halfWidth, halfHeight)
	shape.Radius = radius
	return shape
}

// MakeOffsetBox makes an offset box, bypassing the need for a convex hull
// (upstream b2MakeOffsetBox). center is the local center of the box and
// rotation is the local rotation.
func MakeOffsetBox(halfWidth, halfHeight float64, center Vec2, rotation Rot) Polygon {
	xf := Transform{P: center, Q: rotation}

	var shape Polygon
	shape.Count = 4
	shape.Vertices[0] = TransformPoint(xf, Vec2{-halfWidth, -halfHeight})
	shape.Vertices[1] = TransformPoint(xf, Vec2{halfWidth, -halfHeight})
	shape.Vertices[2] = TransformPoint(xf, Vec2{halfWidth, halfHeight})
	shape.Vertices[3] = TransformPoint(xf, Vec2{-halfWidth, halfHeight})
	shape.Normals[0] = RotateVector(xf.Q, Vec2{0.0, -1.0})
	shape.Normals[1] = RotateVector(xf.Q, Vec2{1.0, 0.0})
	shape.Normals[2] = RotateVector(xf.Q, Vec2{0.0, 1.0})
	shape.Normals[3] = RotateVector(xf.Q, Vec2{-1.0, 0.0})
	shape.Radius = 0.0
	shape.Centroid = xf.P
	return shape
}

// MakeOffsetRoundedBox makes an offset rounded box, bypassing the need for a
// convex hull (upstream b2MakeOffsetRoundedBox).
func MakeOffsetRoundedBox(halfWidth, halfHeight float64, center Vec2, rotation Rot, radius float64) Polygon {
	assert(IsValidFloat(radius) && radius >= 0.0)
	xf := Transform{P: center, Q: rotation}

	var shape Polygon
	shape.Count = 4
	shape.Vertices[0] = TransformPoint(xf, Vec2{-halfWidth, -halfHeight})
	shape.Vertices[1] = TransformPoint(xf, Vec2{halfWidth, -halfHeight})
	shape.Vertices[2] = TransformPoint(xf, Vec2{halfWidth, halfHeight})
	shape.Vertices[3] = TransformPoint(xf, Vec2{-halfWidth, halfHeight})
	shape.Normals[0] = RotateVector(xf.Q, Vec2{0.0, -1.0})
	shape.Normals[1] = RotateVector(xf.Q, Vec2{1.0, 0.0})
	shape.Normals[2] = RotateVector(xf.Q, Vec2{0.0, 1.0})
	shape.Normals[3] = RotateVector(xf.Q, Vec2{-1.0, 0.0})
	shape.Radius = radius
	shape.Centroid = xf.P
	return shape
}

// TransformPolygon transforms a polygon (upstream b2TransformPolygon). This is
// useful for transferring a shape from one body to another.
func TransformPolygon(transform Transform, polygon *Polygon) Polygon {
	p := *polygon

	for i := range p.Count {
		p.Vertices[i] = TransformPoint(transform, p.Vertices[i])
		p.Normals[i] = RotateVector(transform.Q, p.Normals[i])
	}

	p.Centroid = TransformPoint(transform, p.Centroid)

	return p
}

// ComputeCircleMass computes the mass properties of a circle (upstream
// b2ComputeCircleMass).
func ComputeCircleMass(shape *Circle, density float64) MassData {
	rr := shape.Radius * shape.Radius

	var massData MassData
	massData.Mass = density * Pi * rr
	massData.Center = shape.Center

	// inertia about the center of mass
	massData.RotationalInertia = massData.Mass * 0.5 * rr

	return massData
}

// ComputeCapsuleMass computes the mass properties of a capsule (upstream
// b2ComputeCapsuleMass).
func ComputeCapsuleMass(shape *Capsule, density float64) MassData {
	radius := shape.Radius
	rr := radius * radius
	p1 := shape.Center1
	p2 := shape.Center2
	length := Length(Sub(p2, p1))
	ll := length * length

	circleMass := float64(density * (Pi * radius * radius))
	boxMass := float64(density * (2.0 * radius * length))

	var massData MassData
	massData.Mass = circleMass + boxMass
	massData.Center.X = 0.5 * (p1.X + p2.X)
	massData.Center.Y = 0.5 * (p1.Y + p2.Y)

	// two offset half circles, both halves add up to full circle and each half is offset by half length
	// semi-circle centroid = 4 r / 3 pi
	// Need to apply parallel-axis theorem twice:
	// 1. shift semi-circle centroid to origin
	// 2. shift semi-circle to box end
	// m * ((h + lc)^2 - lc^2) = m * (h^2 + 2 * h * lc)
	// See: https://en.wikipedia.org/wiki/Parallel_axis_theorem
	// I verified this formula by computing the convex hull of a 128 vertex capsule

	// half circle centroid
	lc := float64(4.0*radius) / float64(3.0*Pi)

	// half length of rectangular portion of capsule
	h := float64(0.5 * length)

	// 0.5 * rr + h * h + 2.0 * h * lc, rounded per product (see math_fma.go)
	circleInertia := float64(circleMass * (dot2(0.5, rr, h, h) + float64(float64(2.0*h)*lc)))
	boxInertia := float64(boxMass * mulAdd(4.0, rr, ll) / 12.0)
	massData.RotationalInertia = circleInertia + boxInertia

	return massData
}

// ComputePolygonMass computes the mass properties of a polygon (upstream
// b2ComputePolygonMass).
func ComputePolygonMass(shape *Polygon, density float64) MassData {
	// Public API precondition: this function indexes fixed-size vertex and
	// normal arrays with shape.Count, so a hand-built Polygon with an
	// out-of-range Count must fail here rather than below.
	requireValidPolygonCount(shape)

	// Polygon mass, centroid, and inertia.
	// Let rho be the polygon density in mass per unit area.
	// Then:
	// mass = rho * int(dA)
	// centroid.x = (1/mass) * rho * int(x * dA)
	// centroid.y = (1/mass) * rho * int(y * dA)
	// I = rho * int((x*x + y*y) * dA)
	//
	// We can compute these integrals by summing all the integrals
	// for each triangle of the polygon. To evaluate the integral
	// for a single triangle, we make a change of variables to
	// the (u,v) coordinates of the triangle:
	// x = x0 + e1x * u + e2x * v
	// y = y0 + e1y * u + e2y * v
	// where 0 <= u && 0 <= v && u + v <= 1.
	//
	// We integrate u from [0,1-v] and then v from [0,1].
	// We also need to use the Jacobian of the transformation:
	// D = cross(e1, e2)
	//
	// Simplification: triangle centroid = (1/3) * (p1 + p2 + p3)
	//
	// The rest of the derivation is handled by computer algebra.

	assert(shape.Count > 0)

	if shape.Count == 1 {
		var circle Circle
		circle.Center = shape.Vertices[0]
		circle.Radius = shape.Radius
		return ComputeCircleMass(&circle, density)
	}

	if shape.Count == 2 {
		var capsule Capsule
		capsule.Center1 = shape.Vertices[0]
		capsule.Center2 = shape.Vertices[1]
		capsule.Radius = shape.Radius
		return ComputeCapsuleMass(&capsule, density)
	}

	var vertices [MaxPolygonVertices]Vec2
	count := shape.Count
	radius := shape.Radius

	if radius > 0.0 {
		// Approximate mass of rounded polygons by pushing out the vertices.
		sqrt2 := 1.412
		for i := range count {
			j := i - 1
			if i == 0 {
				j = count - 1
			}
			n1 := shape.Normals[j]
			n2 := shape.Normals[i]

			mid := Normalize(Add(n1, n2))
			vertices[i] = MulAdd(shape.Vertices[i], sqrt2*radius, mid)
		}
	} else {
		for i := range count {
			//nolint:gosec // G602: count is shape.Count, validated to 1..MaxPolygonVertices by requireValidPolygonCount at the top of ComputePolygonMass; vertices is [MaxPolygonVertices]Vec2.
			vertices[i] = shape.Vertices[i]
		}
	}

	center := Vec2{0.0, 0.0}
	area := 0.0
	rotationalInertia := 0.0

	// Get a reference point for forming triangles.
	// Use the first vertex to reduce round-off errors.
	//nolint:gosec // G602: vertices is the local [MaxPolygonVertices]Vec2 declared above, so index 0 is always in range.
	r := vertices[0]

	const inv3 float64 = 1.0 / 3.0

	for i := 1; i < count-1; i++ {
		// Triangle edges
		e1 := Sub(vertices[i], r)
		e2 := Sub(vertices[i+1], r)

		d := Cross(e1, e2)

		triangleArea := float64(0.5 * d)
		area += triangleArea

		// Area weighted centroid, r at origin
		center = MulAdd(center, triangleArea*inv3, Add(e1, e2))

		ex1, ey1 := e1.X, e1.Y
		ex2, ey2 := e2.X, e2.Y

		// ex1*ex1 + ex2*ex1 + ex2*ex2, rounded per product (see math_fma.go)
		intx2 := dot2(ex1, ex1, ex2, ex1) + float64(ex2*ex2)
		inty2 := dot2(ey1, ey1, ey2, ey1) + float64(ey2*ey2)

		rotationalInertia = mulAdd(0.25*inv3*d, intx2+inty2, rotationalInertia)
	}

	var massData MassData

	// Total mass
	massData.Mass = density * area

	// Center of mass, shift back from origin at r
	assert(area > epsilon)
	invArea := 1.0 / area
	center.X *= invArea
	center.Y *= invArea
	massData.Center = Add(r, center)

	// Inertia tensor relative to the local origin (point s).
	massData.RotationalInertia = float64(density * rotationalInertia)

	// Shift inertia to center of mass
	massData.RotationalInertia -= float64(massData.Mass * Dot(center, center))

	// If this goes negative we are hosed
	assert(massData.RotationalInertia >= 0.0)

	return massData
}

// ComputeCircleAABB computes the bounding box of a transformed circle
// (upstream b2ComputeCircleAABB).
func ComputeCircleAABB(shape *Circle, xf Transform) AABB {
	p := TransformPoint(xf, shape.Center)
	r := shape.Radius

	aabb := AABB{Vec2{p.X - r, p.Y - r}, Vec2{p.X + r, p.Y + r}}
	return aabb
}

// ComputeCapsuleAABB computes the bounding box of a transformed capsule
// (upstream b2ComputeCapsuleAABB).
func ComputeCapsuleAABB(shape *Capsule, xf Transform) AABB {
	v1 := TransformPoint(xf, shape.Center1)
	v2 := TransformPoint(xf, shape.Center2)

	r := Vec2{shape.Radius, shape.Radius}
	lower := Sub(Min(v1, v2), r)
	upper := Add(Max(v1, v2), r)

	aabb := AABB{lower, upper}
	return aabb
}

// ComputePolygonAABB computes the bounding box of a transformed polygon
// (upstream b2ComputePolygonAABB).
func ComputePolygonAABB(shape *Polygon, xf Transform) AABB {
	assert(shape.Count > 0)
	lower := TransformPoint(xf, shape.Vertices[0])
	upper := lower

	for i := 1; i < shape.Count; i++ {
		v := TransformPoint(xf, shape.Vertices[i])
		lower = Min(lower, v)
		upper = Max(upper, v)
	}

	r := Vec2{shape.Radius, shape.Radius}
	lower = Sub(lower, r)
	upper = Add(upper, r)

	aabb := AABB{lower, upper}
	return aabb
}

// ComputeSegmentAABB computes the bounding box of a transformed line segment
// (upstream b2ComputeSegmentAABB).
func ComputeSegmentAABB(shape *Segment, xf Transform) AABB {
	v1 := TransformPoint(xf, shape.Point1)
	v2 := TransformPoint(xf, shape.Point2)

	lower := Min(v1, v2)
	upper := Max(v1, v2)

	aabb := AABB{lower, upper}
	return aabb
}

// PointInCircle tests a point for overlap with a circle in local space
// (upstream b2PointInCircle).
func PointInCircle(shape *Circle, point Vec2) bool {
	center := shape.Center
	return DistanceSquared(point, center) <= shape.Radius*shape.Radius
}

// PointInCapsule tests a point for overlap with a capsule in local space
// (upstream b2PointInCapsule).
func PointInCapsule(shape *Capsule, point Vec2) bool {
	rr := shape.Radius * shape.Radius
	p1 := shape.Center1
	p2 := shape.Center2

	d := Sub(p2, p1)
	dd := Dot(d, d)
	if dd == 0.0 {
		// Capsule is really a circle
		return DistanceSquared(point, p1) <= rr
	}

	// Get closest point on capsule segment
	// c = p1 + t * d
	// dot(point - c, d) = 0
	// dot(point - p1 - t * d, d) = 0
	// t = dot(point - p1, d) / dot(d, d)
	t := Dot(Sub(point, p1), d) / dd
	t = clampFloat(t, 0.0, 1.0)
	c := MulAdd(p1, t, d)

	// Is query point within radius around closest point?
	return DistanceSquared(point, c) <= rr
}

// PointInPolygon tests a point for overlap with a convex polygon in local
// space (upstream b2PointInPolygon).
func PointInPolygon(shape *Polygon, point Vec2) bool {
	var input DistanceInput
	input.ProxyA = MakeProxy(shape.Vertices[:], shape.Count, 0.0)
	input.ProxyB = MakeProxy([]Vec2{point}, 1, 0.0)
	input.TransformA = TransformIdentity
	input.TransformB = TransformIdentity
	input.UseRadii = false

	var cache SimplexCache
	output := ShapeDistance(&input, &cache, nil)

	return output.Distance <= shape.Radius
}

// RayCastCircle casts a ray against a circle shape in local space (upstream
// b2RayCastCircle).
//
// Precision Improvements for Ray / Sphere Intersection - Ray Tracing Gems 2019
// http://www.codercorner.com/blog/?p=321
func RayCastCircle(shape *Circle, input *RayCastInput) CastOutput {
	if debugAsserts {
		assert(IsValidRay(input))
	}

	p := shape.Center

	var output CastOutput

	// Shift ray so circle center is the origin
	s := Sub(input.Origin, p)

	r := shape.Radius
	rr := r * r

	d, length := GetLengthAndNormalize(input.Translation)
	if length == 0.0 {
		// zero length ray

		if LengthSquared(s) < rr {
			// initial overlap
			output.Point = input.Origin
			output.Hit = true
		}

		return output
	}

	// Find closest point on ray to origin

	// solve: dot(s + t * d, d) = 0
	t := -Dot(s, d)

	// c is the closest point on the line to the origin
	c := MulAdd(s, t, d)

	cc := Dot(c, c)

	if cc > rr {
		// closest point is outside the circle
		return output
	}

	// Pythagoras
	h := math.Sqrt(subF(rr, cc))

	fraction := t - h

	if fraction < 0.0 || input.MaxFraction*length < fraction {
		// intersection is point outside the range of the ray segment

		if LengthSquared(s) < rr {
			// initial overlap
			output.Point = input.Origin
			output.Hit = true
		}

		return output
	}

	// hit point relative to center
	hitPoint := MulAdd(s, fraction, d)

	output.Fraction = fraction / length
	output.Normal = Normalize(hitPoint)
	output.Point = MulAdd(p, shape.Radius, output.Normal)
	output.Hit = true

	return output
}

// RayCastCapsule casts a ray against a capsule shape in local space (upstream
// b2RayCastCapsule).
func RayCastCapsule(shape *Capsule, input *RayCastInput) CastOutput {
	if debugAsserts {
		assert(IsValidRay(input))
	}

	var output CastOutput

	v1 := shape.Center1
	v2 := shape.Center2

	e := Sub(v2, v1)

	a, capsuleLength := GetLengthAndNormalize(e)

	if capsuleLength < epsilon {
		// Capsule is really a circle
		circle := Circle{v1, shape.Radius}
		return RayCastCircle(&circle, input)
	}

	p1 := input.Origin
	d := input.Translation

	// Ray from capsule start to ray start
	q := Sub(p1, v1)
	qa := Dot(q, a)

	// Vector to ray start that is perpendicular to capsule axis
	qp := MulAdd(q, -qa, a)

	radius := shape.Radius

	// Does the ray start within the infinite length capsule?
	if Dot(qp, qp) < radius*radius {
		if qa < 0.0 {
			// start point behind capsule segment
			circle := Circle{v1, shape.Radius}
			return RayCastCircle(&circle, input)
		}

		if qa > capsuleLength {
			// start point ahead of capsule segment
			circle := Circle{v2, shape.Radius}
			return RayCastCircle(&circle, input)
		}

		// ray starts inside capsule -> no hit
		output.Point = input.Origin
		output.Hit = true
		return output
	}

	// Perpendicular to capsule axis, pointing right
	n := Vec2{a.Y, -a.X}

	u, rayLength := GetLengthAndNormalize(d)

	// Intersect ray with infinite length capsule
	// v1 + radius * n + s1 * a = p1 + s2 * u
	// v1 - radius * n + s1 * a = p1 + s2 * u

	// s1 * a - s2 * u = b
	// b = q - radius * ap
	// or
	// b = q + radius * ap

	// Cramer's rule [a -u]
	// upstream: -a.x * u.y + u.x * a.y
	den := cross2(u.X, a.Y, a.X, u.Y)
	if -epsilon < den && den < epsilon {
		// Ray is parallel to capsule and outside infinite length capsule
		return output
	}

	b1 := MulSub(q, radius, n)
	b2 := MulAdd(q, radius, n)

	invDen := 1.0 / den

	// Cramer's rule [a b1]
	s21 := cross2(a.X, b1.Y, b1.X, a.Y) * invDen

	// Cramer's rule [a b2]
	s22 := cross2(a.X, b2.Y, b2.X, a.Y) * invDen

	var s2 float64
	var b Vec2
	if s21 < s22 {
		s2 = s21
		b = b1
	} else {
		s2 = s22
		b = b2
		n = Neg(n)
	}

	if s2 < 0.0 || input.MaxFraction*rayLength < s2 {
		return output
	}

	// Cramer's rule [b -u]
	// upstream: -b.x * u.y + u.x * b.y
	s1 := cross2(u.X, b.Y, b.X, u.Y) * invDen

	switch {
	case s1 < 0.0:
		// ray passes behind capsule segment
		circle := Circle{v1, shape.Radius}
		return RayCastCircle(&circle, input)
	case capsuleLength < s1:
		// ray passes ahead of capsule segment
		circle := Circle{v2, shape.Radius}
		return RayCastCircle(&circle, input)
	default:
		// ray hits capsule side
		output.Fraction = s2 / rayLength
		output.Point = Add(Lerp(v1, v2, s1/capsuleLength), MulSV(shape.Radius, n))
		output.Normal = n
		output.Hit = true
		return output
	}
}

// RayCastSegment casts a ray against a segment shape in local space (upstream
// b2RayCastSegment). Optionally treat the segment as one-sided with hits from
// the left side being treated as a miss.
func RayCastSegment(shape *Segment, input *RayCastInput, oneSided bool) CastOutput {
	if oneSided {
		// Skip left-side collision
		offset := Cross(Sub(input.Origin, shape.Point1), Sub(shape.Point2, shape.Point1))
		if offset < 0.0 {
			var output CastOutput
			return output
		}
	}

	// Put the ray into the edge's frame of reference.
	p1 := input.Origin
	d := input.Translation

	v1 := shape.Point1
	v2 := shape.Point2
	e := Sub(v2, v1)

	var output CastOutput

	eUnit, length := GetLengthAndNormalize(e)
	if length == 0.0 {
		return output
	}

	// Normal points to the right, looking from v1 towards v2
	normal := RightPerp(eUnit)

	// Intersect ray with infinite segment using normal
	// Similar to intersecting a ray with an infinite plane
	// p = p1 + t * d
	// dot(normal, p - v1) = 0
	// dot(normal, p1 - v1) + t * dot(normal, d) = 0
	numerator := Dot(normal, Sub(v1, p1))
	denominator := Dot(normal, d)

	if denominator == 0.0 {
		// parallel
		return output
	}

	t := numerator / denominator
	if t < 0.0 || input.MaxFraction < t {
		// out of ray range
		return output
	}

	// Intersection point on infinite segment
	p := MulAdd(p1, t, d)

	// Compute position of p along segment
	// p = v1 + s * e
	// s = dot(p - v1, e) / dot(e, e)

	s := Dot(Sub(p, v1), eUnit)
	if s < 0.0 || length < s {
		// out of segment range
		return output
	}

	if numerator > 0.0 {
		normal = Neg(normal)
	}

	output.Fraction = t
	output.Point = p
	output.Normal = normal
	output.Hit = true

	return output
}

// RayCastPolygon casts a ray against a polygon shape in local space (upstream
// b2RayCastPolygon).
func RayCastPolygon(shape *Polygon, input *RayCastInput) CastOutput {
	if debugAsserts {
		assert(IsValidRay(input))
	}

	if shape.Radius == 0.0 {
		// Shift all math to first vertex since the polygon may be far
		// from the origin.
		base := shape.Vertices[0]

		p1 := Sub(input.Origin, base)
		d := input.Translation

		lower, upper := 0.0, input.MaxFraction

		index := -1

		var output CastOutput

		for edgeIndex := range shape.Count {
			// p = p1 + a * d
			// dot(normal, p - v) = 0
			// dot(normal, p1 - v) + a * dot(normal, d) = 0
			vertex := Sub(shape.Vertices[edgeIndex], base)
			numerator := Dot(shape.Normals[edgeIndex], Sub(vertex, p1))
			denominator := Dot(shape.Normals[edgeIndex], d)

			if denominator == 0.0 {
				// Parallel and runs outside edge
				if numerator < 0.0 {
					return output
				}
			} else {
				// Note: we want this predicate without division:
				// lower < numerator / denominator, where denominator < 0
				// Since denominator < 0, we have to flip the inequality:
				// lower < numerator / denominator <==> denominator * lower > numerator.
				if denominator < 0.0 && numerator < lower*denominator {
					// Increase lower.
					// The segment enters this half-space.
					lower = numerator / denominator
					index = edgeIndex
				} else if denominator > 0.0 && numerator < upper*denominator {
					// Decrease upper.
					// The segment exits this half-space.
					upper = numerator / denominator
				}
			}

			if upper < lower {
				// Ray misses
				return output
			}
		}

		assert(0.0 <= lower && lower <= input.MaxFraction)

		if index >= 0 {
			output.Fraction = lower
			output.Normal = shape.Normals[index]
			output.Point = MulAdd(input.Origin, lower, d)
			output.Hit = true
		} else {
			// initial overlap
			output.Point = input.Origin
			output.Hit = true
		}

		return output
	}

	var castInput ShapeCastPairInput
	castInput.ProxyA = MakeProxy(shape.Vertices[:], shape.Count, shape.Radius)
	castInput.ProxyB = MakeProxy([]Vec2{input.Origin}, 1, 0.0)
	castInput.TransformA = TransformIdentity
	castInput.TransformB = TransformIdentity
	castInput.TranslationB = input.Translation
	castInput.MaxFraction = input.MaxFraction
	castInput.CanEncroach = false
	return ShapeCast(&castInput)
}

// ShapeCastCircle performs a shape cast against a circle in local space
// (upstream b2ShapeCastCircle).
func ShapeCastCircle(shape *Circle, input *ShapeCastInput) CastOutput {
	var pairInput ShapeCastPairInput
	pairInput.ProxyA = MakeProxy([]Vec2{shape.Center}, 1, shape.Radius)
	pairInput.ProxyB = input.Proxy
	pairInput.TransformA = TransformIdentity
	pairInput.TransformB = TransformIdentity
	pairInput.TranslationB = input.Translation
	pairInput.MaxFraction = input.MaxFraction
	pairInput.CanEncroach = input.CanEncroach

	output := ShapeCast(&pairInput)
	return output
}

// ShapeCastCapsule performs a shape cast against a capsule in local space
// (upstream b2ShapeCastCapsule).
func ShapeCastCapsule(shape *Capsule, input *ShapeCastInput) CastOutput {
	var pairInput ShapeCastPairInput
	pairInput.ProxyA = MakeProxy([]Vec2{shape.Center1, shape.Center2}, 2, shape.Radius)
	pairInput.ProxyB = input.Proxy
	pairInput.TransformA = TransformIdentity
	pairInput.TransformB = TransformIdentity
	pairInput.TranslationB = input.Translation
	pairInput.MaxFraction = input.MaxFraction
	pairInput.CanEncroach = input.CanEncroach

	output := ShapeCast(&pairInput)
	return output
}

// ShapeCastSegment performs a shape cast against a segment in local space
// (upstream b2ShapeCastSegment).
func ShapeCastSegment(shape *Segment, input *ShapeCastInput) CastOutput {
	var pairInput ShapeCastPairInput
	pairInput.ProxyA = MakeProxy([]Vec2{shape.Point1, shape.Point2}, 2, 0.0)
	pairInput.ProxyB = input.Proxy
	pairInput.TransformA = TransformIdentity
	pairInput.TransformB = TransformIdentity
	pairInput.TranslationB = input.Translation
	pairInput.MaxFraction = input.MaxFraction
	pairInput.CanEncroach = input.CanEncroach

	output := ShapeCast(&pairInput)
	return output
}

// ShapeCastPolygon performs a shape cast against a convex polygon in local
// space (upstream b2ShapeCastPolygon).
func ShapeCastPolygon(shape *Polygon, input *ShapeCastInput) CastOutput {
	var pairInput ShapeCastPairInput
	pairInput.ProxyA = MakeProxy(shape.Vertices[:], shape.Count, shape.Radius)
	pairInput.ProxyB = input.Proxy
	pairInput.TransformA = TransformIdentity
	pairInput.TransformB = TransformIdentity
	pairInput.TranslationB = input.Translation
	pairInput.MaxFraction = input.MaxFraction
	pairInput.CanEncroach = input.CanEncroach

	output := ShapeCast(&pairInput)
	return output
}

// CollideMoverAndCircle collides a capsule mover with a circle (upstream
// b2CollideMoverAndCircle).
func CollideMoverAndCircle(mover *Capsule, shape *Circle) PlaneResult {
	var distanceInput DistanceInput
	distanceInput.ProxyA = MakeProxy([]Vec2{shape.Center}, 1, 0.0)
	distanceInput.ProxyB = MakeProxy([]Vec2{mover.Center1, mover.Center2}, 2, mover.Radius)
	distanceInput.TransformA = TransformIdentity
	distanceInput.TransformB = TransformIdentity
	distanceInput.UseRadii = false

	totalRadius := mover.Radius + shape.Radius

	var cache SimplexCache
	distanceOutput := ShapeDistance(&distanceInput, &cache, nil)

	if distanceOutput.Distance <= totalRadius {
		plane := Plane{distanceOutput.Normal, totalRadius - distanceOutput.Distance}
		return PlaneResult{
			Plane: plane,
			Point: distanceOutput.PointA,
			Hit:   true,
		}
	}

	return PlaneResult{}
}

// CollideMoverAndCapsule collides a capsule mover with a capsule (upstream
// b2CollideMoverAndCapsule).
func CollideMoverAndCapsule(mover *Capsule, shape *Capsule) PlaneResult {
	var distanceInput DistanceInput
	distanceInput.ProxyA = MakeProxy([]Vec2{shape.Center1, shape.Center2}, 2, 0.0)
	distanceInput.ProxyB = MakeProxy([]Vec2{mover.Center1, mover.Center2}, 2, mover.Radius)
	distanceInput.TransformA = TransformIdentity
	distanceInput.TransformB = TransformIdentity
	distanceInput.UseRadii = false

	totalRadius := mover.Radius + shape.Radius

	var cache SimplexCache
	distanceOutput := ShapeDistance(&distanceInput, &cache, nil)

	if distanceOutput.Distance <= totalRadius {
		plane := Plane{distanceOutput.Normal, totalRadius - distanceOutput.Distance}
		return PlaneResult{
			Plane: plane,
			Point: distanceOutput.PointA,
			Hit:   true,
		}
	}

	return PlaneResult{}
}

// CollideMoverAndPolygon collides a capsule mover with a convex polygon
// (upstream b2CollideMoverAndPolygon).
func CollideMoverAndPolygon(mover *Capsule, shape *Polygon) PlaneResult {
	var distanceInput DistanceInput
	distanceInput.ProxyA = MakeProxy(shape.Vertices[:], shape.Count, shape.Radius)
	distanceInput.ProxyB = MakeProxy([]Vec2{mover.Center1, mover.Center2}, 2, mover.Radius)
	distanceInput.TransformA = TransformIdentity
	distanceInput.TransformB = TransformIdentity
	distanceInput.UseRadii = false

	totalRadius := mover.Radius + shape.Radius

	var cache SimplexCache
	distanceOutput := ShapeDistance(&distanceInput, &cache, nil)

	if distanceOutput.Distance <= totalRadius {
		plane := Plane{distanceOutput.Normal, totalRadius - distanceOutput.Distance}
		return PlaneResult{
			Plane: plane,
			Point: distanceOutput.PointA,
			Hit:   true,
		}
	}

	return PlaneResult{}
}

// CollideMoverAndSegment collides a capsule mover with a segment (upstream
// b2CollideMoverAndSegment).
func CollideMoverAndSegment(mover *Capsule, shape *Segment) PlaneResult {
	var distanceInput DistanceInput
	distanceInput.ProxyA = MakeProxy([]Vec2{shape.Point1, shape.Point2}, 2, 0.0)
	distanceInput.ProxyB = MakeProxy([]Vec2{mover.Center1, mover.Center2}, 2, mover.Radius)
	distanceInput.TransformA = TransformIdentity
	distanceInput.TransformB = TransformIdentity
	distanceInput.UseRadii = false

	totalRadius := mover.Radius

	var cache SimplexCache
	distanceOutput := ShapeDistance(&distanceInput, &cache, nil)

	if distanceOutput.Distance <= totalRadius {
		plane := Plane{distanceOutput.Normal, totalRadius - distanceOutput.Distance}
		return PlaneResult{
			Plane: plane,
			Point: distanceOutput.PointA,
			Hit:   true,
		}
	}

	return PlaneResult{}
}
