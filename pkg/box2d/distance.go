// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/distance.c.
// This port uses float64 where upstream uses float; all multiply-accumulate
// expressions are explicitly rounded (see math_fma.go).
//
// The upstream file also contains b2ShapeCastMerged inside an `#if 0` block
// (dead experimental GJK-raycast) and the B2_SNOOP_TOI_COUNTERS globals inside
// disabled preprocessor blocks; neither is ported.

package box2d

// GetSweepTransform evaluates the transform sweep at a specific time (upstream
// b2GetSweepTransform).
func GetSweepTransform(sweep *Sweep, time float64) Transform {
	// https://fgiesen.wordpress.com/2012/08/15/linear-interpolation-past-present-and-future/
	var xf Transform
	xf.P = Add(MulSV(1.0-time, sweep.C1), MulSV(time, sweep.C2))

	// upstream: (1.0f - time) * sweep->q1.c + time * sweep->q2.c (and .s)
	q := Rot{
		dot2(1.0-time, sweep.Q1.C, time, sweep.Q2.C),
		dot2(1.0-time, sweep.Q1.S, time, sweep.Q2.S),
	}

	xf.Q = NormalizeRot(q)

	// Shift to origin
	xf.P = Sub(xf.P, RotateVector(xf.Q, sweep.LocalCenter))
	return xf
}

// SegmentDistance computes the distance between two line segments, clamping at
// the end points if needed (upstream b2SegmentDistance).
//
// Follows Ericson 5.1.9 Closest Points of Two Line Segments.
func SegmentDistance(p1, q1, p2, q2 Vec2) SegmentDistanceResult {
	var result SegmentDistanceResult

	d1 := Sub(q1, p1)
	d2 := Sub(q2, p2)
	r := Sub(p1, p2)
	dd1 := Dot(d1, d1)
	dd2 := Dot(d2, d2)
	rd1 := Dot(r, d1)
	rd2 := Dot(r, d2)

	const epsSqr = epsilon * epsilon

	if dd1 < epsSqr || dd2 < epsSqr {
		// Handle all degeneracies (upstream if/else-if/else chain; branch
		// order preserved).
		switch {
		case dd1 >= epsSqr:
			// Segment 2 is degenerate
			result.Fraction1 = clampFloat(-rd1/dd1, 0.0, 1.0)
			result.Fraction2 = 0.0
		case dd2 >= epsSqr:
			// Segment 1 is degenerate
			result.Fraction1 = 0.0
			result.Fraction2 = clampFloat(rd2/dd2, 0.0, 1.0)
		default:
			result.Fraction1 = 0.0
			result.Fraction2 = 0.0
		}
	} else {
		// Non-degenerate segments
		d12 := Dot(d1, d2)

		// upstream: dd1 * dd2 - d12 * d12
		denominator := cross2(dd1, dd2, d12, d12)

		// Fraction on segment 1
		f1 := 0.0
		if denominator != 0.0 {
			// not parallel
			// upstream: ( d12 * rd2 - rd1 * dd2 ) / denominator
			f1 = clampFloat(cross2(d12, rd2, rd1, dd2)/denominator, 0.0, 1.0)
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

		result.Fraction1 = f1
		result.Fraction2 = f2
	}

	result.Closest1 = MulAdd(p1, result.Fraction1, d1)
	result.Closest2 = MulAdd(p2, result.Fraction2, d2)
	result.DistanceSquared = DistanceSquared(result.Closest1, result.Closest2)
	return result
}

// MakeProxy makes a proxy for use in overlap, shape cast, and related
// functions (upstream b2MakeProxy). This is a deep copy of the points.
func MakeProxy(points []Vec2, count int, radius float64) ShapeProxy {
	count = minInt(count, MaxPolygonVertices)
	var proxy ShapeProxy
	for i := range count {
		proxy.Points[i] = points[i]
	}
	proxy.Count = count
	proxy.Radius = radius
	return proxy
}

// MakeOffsetProxy makes a proxy with a transform (upstream b2MakeOffsetProxy).
// This is a deep copy of the points.
func MakeOffsetProxy(points []Vec2, count int, radius float64, position Vec2, rotation Rot) ShapeProxy {
	count = minInt(count, MaxPolygonVertices)
	transform := Transform{
		P: position,
		Q: rotation,
	}
	var proxy ShapeProxy
	for i := range count {
		proxy.Points[i] = TransformPoint(transform, points[i])
	}
	proxy.Count = count
	proxy.Radius = radius
	return proxy
}

// weight2 is upstream b2Weight2.
func weight2(a1 float64, w1 Vec2, a2 float64, w2 Vec2) Vec2 {
	// upstream: a1 * w1.x + a2 * w2.x (and .y)
	return Vec2{dot2(a1, w1.X, a2, w2.X), dot2(a1, w1.Y, a2, w2.Y)}
}

// weight3 is upstream b2Weight3.
func weight3(a1 float64, w1 Vec2, a2 float64, w2 Vec2, a3 float64, w3 Vec2) Vec2 {
	// upstream: a1 * w1.x + a2 * w2.x + a3 * w3.x (and .y), left associative
	return Vec2{
		dot2(a1, w1.X, a2, w2.X) + float64(a3*w3.X),
		dot2(a1, w1.Y, a2, w2.Y) + float64(a3*w3.Y),
	}
}

// findSupport is upstream b2FindSupport.
func findSupport(proxy *ShapeProxy, direction Vec2) int {
	points := &proxy.Points
	count := proxy.Count

	bestIndex := 0
	bestValue := Dot(points[0], direction)
	for i := 1; i < count; i++ {
		value := Dot(points[i], direction)
		if value > bestValue {
			bestIndex = i
			bestValue = value
		}
	}

	return bestIndex
}

// makeSimplexFromCache is upstream b2MakeSimplexFromCache.
func makeSimplexFromCache(cache SimplexCache, proxyA, proxyB *ShapeProxy) Simplex {
	assert(cache.Count <= 3)
	var s Simplex

	// Copy data from cache.
	s.Count = int(cache.Count)

	vertices := [3]*SimplexVertex{&s.V1, &s.V2, &s.V3}
	for i := range s.Count {
		v := vertices[i]
		v.IndexA = int(cache.IndexA[i])
		v.IndexB = int(cache.IndexB[i])
		v.WA = proxyA.Points[v.IndexA]
		v.WB = proxyB.Points[v.IndexB]
		v.W = Sub(v.WA, v.WB)

		// invalid
		v.A = -1.0
	}

	// If the cache is empty or invalid ...
	if s.Count == 0 {
		v := vertices[0]
		v.IndexA = 0
		v.IndexB = 0
		v.WA = proxyA.Points[0]
		v.WB = proxyB.Points[0]
		v.W = Sub(v.WA, v.WB)
		v.A = 1.0
		s.Count = 1
	}

	return s
}

// makeSimplexCache is upstream b2MakeSimplexCache.
func makeSimplexCache(simplex *Simplex) SimplexCache {
	var cache SimplexCache
	cache.Count = uint16(simplex.Count)
	vertices := [3]*SimplexVertex{&simplex.V1, &simplex.V2, &simplex.V3}
	for i := range simplex.Count {
		cache.IndexA[i] = uint8(vertices[i].IndexA)
		cache.IndexB[i] = uint8(vertices[i].IndexB)
	}

	return cache
}

// computeWitnessPoints is upstream b2ComputeWitnessPoints.
func computeWitnessPoints(s *Simplex, a, b *Vec2) {
	switch s.Count {
	case 1:
		*a = s.V1.WA
		*b = s.V1.WB

	case 2:
		*a = weight2(s.V1.A, s.V1.WA, s.V2.A, s.V2.WA)
		*b = weight2(s.V1.A, s.V1.WB, s.V2.A, s.V2.WB)

	case 3:
		*a = weight3(s.V1.A, s.V1.WA, s.V2.A, s.V2.WA, s.V3.A, s.V3.WA)
		// todo why are these not equal?
		// *b = b2Weight3(s->v1.a, s->v1.wB, s->v2.a, s->v2.wB, s->v3.a, s->v3.wB);
		*b = *a

	default:
		*a = Vec2Zero
		*b = Vec2Zero
		assert(false)
	}
}

// solveSimplex2 solves a line segment using barycentric coordinates (upstream
// b2SolveSimplex2).
//
//	p = a1 * w1 + a2 * w2
//	a1 + a2 = 1
//
// The vector from the origin to the closest point on the line is
// perpendicular to the line.
//
//	e12 = w2 - w1
//	dot(p, e) = 0
//	a1 * dot(w1, e) + a2 * dot(w2, e) = 0
//
// 2-by-2 linear system
//
//	[1      1     ][a1] = [1]
//	[w1.e12 w2.e12][a2] = [0]
//
// Define
//
//	d12_1 =  dot(w2, e12)
//	d12_2 = -dot(w1, e12)
//	d12 = d12_1 + d12_2
//
// Solution
//
//	a1 = d12_1 / d12
//	a2 = d12_2 / d12
//
// It returns a vector that points towards the origin.
func solveSimplex2(s *Simplex) Vec2 {
	w1 := s.V1.W
	w2 := s.V2.W
	e12 := Sub(w2, w1)

	// w1 region (upstream d12_2)
	d122 := -Dot(w1, e12)
	if d122 <= 0.0 {
		// a2 <= 0, so we clamp it to 0
		s.V1.A = 1.0
		s.Count = 1
		return Neg(w1)
	}

	// w2 region (upstream d12_1)
	d121 := Dot(w2, e12)
	if d121 <= 0.0 {
		// a1 <= 0, so we clamp it to 0
		s.V2.A = 1.0
		s.Count = 1
		s.V1 = s.V2
		return Neg(w2)
	}

	// Must be in e12 region.
	invD12 := 1.0 / (d121 + d122)
	s.V1.A = d121 * invD12
	s.V2.A = d122 * invD12
	s.Count = 2
	return CrossSV(Cross(Add(w1, w2), e12), e12)
}

// solveSimplex3 is upstream b2SolveSimplex3.
func solveSimplex3(s *Simplex) Vec2 {
	w1 := s.V1.W
	w2 := s.V2.W
	w3 := s.V3.W

	// Edge12
	// [1      1     ][a1] = [1]
	// [w1.e12 w2.e12][a2] = [0]
	// a3 = 0
	e12 := Sub(w2, w1)
	w1e12 := Dot(w1, e12)
	w2e12 := Dot(w2, e12)
	d121 := w2e12  // upstream d12_1
	d122 := -w1e12 // upstream d12_2

	// Edge13
	// [1      1     ][a1] = [1]
	// [w1.e13 w3.e13][a3] = [0]
	// a2 = 0
	e13 := Sub(w3, w1)
	w1e13 := Dot(w1, e13)
	w3e13 := Dot(w3, e13)
	d131 := w3e13  // upstream d13_1
	d132 := -w1e13 // upstream d13_2

	// Edge23
	// [1      1     ][a2] = [1]
	// [w2.e23 w3.e23][a3] = [0]
	// a1 = 0
	e23 := Sub(w3, w2)
	w2e23 := Dot(w2, e23)
	w3e23 := Dot(w3, e23)
	d231 := w3e23  // upstream d23_1
	d232 := -w2e23 // upstream d23_2

	// Triangle123
	n123 := Cross(e12, e13)

	d1231 := float64(n123 * Cross(w2, w3)) // upstream d123_1
	d1232 := float64(n123 * Cross(w3, w1)) // upstream d123_2
	d1233 := float64(n123 * Cross(w1, w2)) // upstream d123_3

	// w1 region
	if d122 <= 0.0 && d132 <= 0.0 {
		s.V1.A = 1.0
		s.Count = 1
		return Neg(w1)
	}

	// e12
	if d121 > 0.0 && d122 > 0.0 && d1233 <= 0.0 {
		invD12 := 1.0 / (d121 + d122)
		s.V1.A = d121 * invD12
		s.V2.A = d122 * invD12
		s.Count = 2
		return CrossSV(Cross(Add(w1, w2), e12), e12)
	}

	// e13
	if d131 > 0.0 && d132 > 0.0 && d1232 <= 0.0 {
		invD13 := 1.0 / (d131 + d132)
		s.V1.A = d131 * invD13
		s.V3.A = d132 * invD13
		s.Count = 2
		s.V2 = s.V3
		return CrossSV(Cross(Add(w1, w3), e13), e13)
	}

	// w2 region
	if d121 <= 0.0 && d232 <= 0.0 {
		s.V2.A = 1.0
		s.Count = 1
		s.V1 = s.V2
		return Neg(w2)
	}

	// w3 region
	if d131 <= 0.0 && d231 <= 0.0 {
		s.V3.A = 1.0
		s.Count = 1
		s.V1 = s.V3
		return Neg(w3)
	}

	// e23
	if d231 > 0.0 && d232 > 0.0 && d1231 <= 0.0 {
		invD23 := 1.0 / (d231 + d232)
		s.V2.A = d231 * invD23
		s.V3.A = d232 * invD23
		s.Count = 2
		s.V1 = s.V3
		return CrossSV(Cross(Add(w2, w3), e23), e23)
	}

	// Must be in triangle123
	invD123 := 1.0 / (d1231 + d1232 + d1233)
	s.V1.A = d1231 * invD123
	s.V2.A = d1232 * invD123
	s.V3.A = d1233 * invD123
	s.Count = 3

	// No search direction
	return Vec2Zero
}

// ShapeDistance computes the closest points between two shapes represented as
// point clouds (upstream b2ShapeDistance). The SimplexCache cache is
// input/output. On the first call set SimplexCache.Count to zero. The
// underlying GJK algorithm may be debugged by passing in a debug simplexes
// slice. You may pass in nil.
//
// Uses GJK for computing the distance between convex shapes.
// https://box2d.org/files/ErinCatto_GJK_GDC2010.pdf
func ShapeDistance(input *DistanceInput, cache *SimplexCache, simplexes []Simplex) DistanceOutput {
	assert(input.ProxyA.Count > 0 && input.ProxyB.Count > 0)
	assert(input.ProxyA.Radius >= 0.0)
	assert(input.ProxyB.Radius >= 0.0)

	var output DistanceOutput

	proxyA := &input.ProxyA

	// Get proxyB in frame A to avoid further transforms in the main loop.
	// This is still a performance gain at 8 points.
	var localProxyB ShapeProxy
	{
		transform := InvMulTransforms(input.TransformA, input.TransformB)
		localProxyB.Count = input.ProxyB.Count
		localProxyB.Radius = input.ProxyB.Radius
		for i := range localProxyB.Count {
			localProxyB.Points[i] = TransformPoint(transform, input.ProxyB.Points[i])
		}
	}

	// Initialize the simplex.
	simplex := makeSimplexFromCache(*cache, proxyA, &localProxyB)

	simplexIndex := 0
	if simplexes != nil && simplexIndex < len(simplexes) {
		simplexes[simplexIndex] = simplex
		simplexIndex++
	}

	// Get simplex vertices as an array.
	vertices := [3]*SimplexVertex{&simplex.V1, &simplex.V2, &simplex.V3}

	nonUnitNormal := Vec2Zero

	// These store the vertices of the last simplex so that we can check for
	// duplicates and prevent cycling.
	var saveA, saveB [3]int

	// Main iteration loop. All computations are done in frame A.
	const maxIterations = 20
	iteration := 0
	for iteration < maxIterations {
		// Copy simplex so we can identify duplicates.
		saveCount := simplex.Count
		for i := range saveCount {
			saveA[i] = vertices[i].IndexA
			saveB[i] = vertices[i].IndexB
		}

		var d Vec2
		switch simplex.Count {
		case 1:
			d = Neg(simplex.V1.W)

		case 2:
			d = solveSimplex2(&simplex)

		case 3:
			d = solveSimplex3(&simplex)

		default:
			assert(false)
		}

		// If we have 3 points, then the origin is in the corresponding triangle.
		if simplex.Count == 3 {
			// Overlap
			var localPointA, localPointB Vec2
			computeWitnessPoints(&simplex, &localPointA, &localPointB)
			output.PointA = TransformPoint(input.TransformA, localPointA)
			output.PointB = TransformPoint(input.TransformA, localPointB)
			return output
		}

		// upstream: guarded by #ifndef NDEBUG; debugAsserts mirrors a release
		// build where these debug stores are compiled out.
		if debugAsserts && simplexes != nil && simplexIndex < len(simplexes) {
			simplexes[simplexIndex] = simplex
			simplexIndex++
		}

		// Ensure the search direction is numerically fit.
		if Dot(d, d) < epsilon*epsilon {
			// This is unlikely but could lead to bad cycling.
			// The branch predictor seems to make this check have low cost.

			// The origin is probably contained by a line segment
			// or triangle. Thus the shapes are overlapped.

			// Must return overlap due to invalid normal.
			var localPointA, localPointB Vec2
			computeWitnessPoints(&simplex, &localPointA, &localPointB)
			output.PointA = TransformPoint(input.TransformA, localPointA)
			output.PointB = TransformPoint(input.TransformA, localPointB)
			return output
		}

		// Save the normal
		nonUnitNormal = d

		// Compute a tentative new simplex vertex using support points.
		// support = support(a, d) - support(b, -d)
		vertex := vertices[simplex.Count]
		vertex.IndexA = findSupport(proxyA, d)
		vertex.WA = proxyA.Points[vertex.IndexA]
		vertex.IndexB = findSupport(&localProxyB, Neg(d))
		vertex.WB = localProxyB.Points[vertex.IndexB]
		vertex.W = Sub(vertex.WA, vertex.WB)

		// Iteration count is equated to the number of support point calls.
		iteration++

		// Check for duplicate support points. This is the main termination criteria.
		duplicate := false
		for i := range saveCount {
			if vertex.IndexA == saveA[i] && vertex.IndexB == saveB[i] {
				duplicate = true
				break
			}
		}

		// If we found a duplicate support point we must exit to avoid cycling.
		if duplicate {
			break
		}

		// New vertex is valid and needed.
		simplex.Count++
	}

	// upstream: guarded by #ifndef NDEBUG (see above).
	if debugAsserts && simplexes != nil && simplexIndex < len(simplexes) {
		simplexes[simplexIndex] = simplex
		simplexIndex++
	}

	// Prepare output
	normal := Normalize(nonUnitNormal)
	assert(IsNormalized(normal))
	normal = RotateVector(input.TransformA.Q, normal)

	var localPointA, localPointB Vec2
	computeWitnessPoints(&simplex, &localPointA, &localPointB)
	output.Normal = normal
	output.Distance = Distance(localPointA, localPointB)
	output.PointA = TransformPoint(input.TransformA, localPointA)
	output.PointB = TransformPoint(input.TransformA, localPointB)
	output.Iterations = iteration
	output.SimplexCount = simplexIndex

	// Cache the simplex
	*cache = makeSimplexCache(&simplex)

	// Apply radii if requested
	if input.UseRadii {
		radiusA := input.ProxyA.Radius
		radiusB := input.ProxyB.Radius
		output.Distance = maxFloat(0.0, output.Distance-radiusA-radiusB)

		// Keep closest points on perimeter even if overlapped, this way the
		// points move smoothly.
		output.PointA = MulAdd(output.PointA, radiusA, normal)
		output.PointB = MulSub(output.PointB, radiusB, normal)
	}

	return output
}

// ShapeCast performs a linear shape cast of shape B moving and shape A fixed
// using conservative advancement (upstream b2ShapeCast). It determines the hit
// point, normal, and translation fraction. Initially touching shapes are
// treated as a miss.
func ShapeCast(input *ShapeCastPairInput) CastOutput {
	// Compute tolerance
	linearSlop := LinearSlop
	totalRadius := input.ProxyA.Radius + input.ProxyB.Radius
	target := maxFloat(linearSlop, totalRadius-linearSlop)
	tolerance := float64(0.25 * linearSlop)

	assert(target > tolerance)

	// Prepare input for distance query
	var cache SimplexCache

	fraction := 0.0

	var distanceInput DistanceInput
	distanceInput.ProxyA = input.ProxyA
	distanceInput.ProxyB = input.ProxyB
	distanceInput.TransformA = input.TransformA
	distanceInput.TransformB = input.TransformB
	distanceInput.UseRadii = false

	delta2 := input.TranslationB
	var output CastOutput

	iteration := 0
	const maxIterations = 20

	for ; iteration < maxIterations; iteration++ {
		output.Iterations++

		distanceOutput := ShapeDistance(&distanceInput, &cache, nil)

		if distanceOutput.Distance < target+tolerance {
			if iteration == 0 {
				if input.CanEncroach && distanceOutput.Distance > 2.0*linearSlop {
					target = distanceOutput.Distance - linearSlop
				} else {
					// Initial overlap
					output.Hit = true

					// Compute a common point
					c1 := MulAdd(distanceOutput.PointA, input.ProxyA.Radius, distanceOutput.Normal)
					c2 := MulAdd(distanceOutput.PointB, -input.ProxyB.Radius, distanceOutput.Normal)
					output.Point = Lerp(c1, c2, 0.5)
					return output
				}
			} else {
				// Regular hit
				assert(distanceOutput.Distance > 0.0 && IsNormalized(distanceOutput.Normal))
				output.Fraction = fraction
				output.Point = MulAdd(distanceOutput.PointA, input.ProxyA.Radius, distanceOutput.Normal)
				output.Normal = distanceOutput.Normal
				output.Hit = true
				return output
			}
		}

		assert(distanceOutput.Distance > 0.0)
		assert(IsNormalized(distanceOutput.Normal))

		// Check if shapes are approaching each other
		denominator := Dot(delta2, distanceOutput.Normal)
		if denominator >= 0.0 {
			// Miss
			return output
		}

		// Advance sweep
		fraction += (target - distanceOutput.Distance) / denominator
		if fraction >= input.MaxFraction {
			// Miss
			return output
		}

		distanceInput.TransformB.P = MulAdd(input.TransformB.P, fraction, delta2)
	}

	// Failure!
	return output
}

// separationType is upstream b2SeparationType.
type separationType int

// separationType values (upstream b2_pointsType, b2_faceAType, b2_faceBType).
const (
	pointsType separationType = iota
	faceAType
	faceBType
)

// separationFunction is upstream b2SeparationFunction.
type separationFunction struct {
	proxyA     *ShapeProxy
	proxyB     *ShapeProxy
	sweepA     Sweep
	sweepB     Sweep
	localPoint Vec2
	axis       Vec2
	typ        separationType
}

// makeSeparationFunction is upstream b2MakeSeparationFunction.
func makeSeparationFunction(cache SimplexCache, proxyA *ShapeProxy, sweepA *Sweep,
	proxyB *ShapeProxy, sweepB *Sweep, t1 float64,
) separationFunction {
	var f separationFunction

	f.proxyA = proxyA
	f.proxyB = proxyB
	count := int(cache.Count)
	assert(0 < count && count < 3)

	f.sweepA = *sweepA
	f.sweepB = *sweepB

	xfA := GetSweepTransform(sweepA, t1)
	xfB := GetSweepTransform(sweepB, t1)

	if count == 1 {
		f.typ = pointsType
		localPointA := proxyA.Points[cache.IndexA[0]]
		localPointB := proxyB.Points[cache.IndexB[0]]
		pointA := TransformPoint(xfA, localPointA)
		pointB := TransformPoint(xfB, localPointB)
		f.axis = Normalize(Sub(pointB, pointA))
		f.localPoint = Vec2Zero
		return f
	}

	if cache.IndexA[0] == cache.IndexA[1] {
		// Two points on B and one on A.
		f.typ = faceBType
		localPointB1 := proxyB.Points[cache.IndexB[0]]
		localPointB2 := proxyB.Points[cache.IndexB[1]]

		f.axis = CrossVS(Sub(localPointB2, localPointB1), 1.0)
		f.axis = Normalize(f.axis)
		normal := RotateVector(xfB.Q, f.axis)

		f.localPoint = Vec2{0.5 * (localPointB1.X + localPointB2.X), 0.5 * (localPointB1.Y + localPointB2.Y)}
		pointB := TransformPoint(xfB, f.localPoint)

		localPointA := proxyA.Points[cache.IndexA[0]]
		pointA := TransformPoint(xfA, localPointA)

		s := Dot(Sub(pointA, pointB), normal)
		if s < 0.0 {
			f.axis = Neg(f.axis)
		}
		return f
	}

	// Two points on A and one or two points on B.
	f.typ = faceAType
	localPointA1 := proxyA.Points[cache.IndexA[0]]
	localPointA2 := proxyA.Points[cache.IndexA[1]]

	f.axis = CrossVS(Sub(localPointA2, localPointA1), 1.0)
	f.axis = Normalize(f.axis)
	normal := RotateVector(xfA.Q, f.axis)

	f.localPoint = Vec2{0.5 * (localPointA1.X + localPointA2.X), 0.5 * (localPointA1.Y + localPointA2.Y)}
	pointA := TransformPoint(xfA, f.localPoint)

	localPointB := proxyB.Points[cache.IndexB[0]]
	pointB := TransformPoint(xfB, localPointB)

	s := Dot(Sub(pointB, pointA), normal)
	if s < 0.0 {
		f.axis = Neg(f.axis)
	}
	return f
}

// findMinSeparation is upstream b2FindMinSeparation. It returns the minimum
// separation and the witness point indices on A and B (upstream out
// parameters indexA/indexB).
func findMinSeparation(f *separationFunction, t float64) (float64, int, int) {
	xfA := GetSweepTransform(&f.sweepA, t)
	xfB := GetSweepTransform(&f.sweepB, t)

	switch f.typ {
	case pointsType:
		axisA := InvRotateVector(xfA.Q, f.axis)
		axisB := InvRotateVector(xfB.Q, Neg(f.axis))

		indexA := findSupport(f.proxyA, axisA)
		indexB := findSupport(f.proxyB, axisB)

		localPointA := f.proxyA.Points[indexA]
		localPointB := f.proxyB.Points[indexB]

		pointA := TransformPoint(xfA, localPointA)
		pointB := TransformPoint(xfB, localPointB)

		separation := Dot(Sub(pointB, pointA), f.axis)
		return separation, indexA, indexB

	case faceAType:
		normal := RotateVector(xfA.Q, f.axis)
		pointA := TransformPoint(xfA, f.localPoint)

		axisB := InvRotateVector(xfB.Q, Neg(normal))

		indexA := -1
		indexB := findSupport(f.proxyB, axisB)

		localPointB := f.proxyB.Points[indexB]
		pointB := TransformPoint(xfB, localPointB)

		separation := Dot(Sub(pointB, pointA), normal)
		return separation, indexA, indexB

	case faceBType:
		normal := RotateVector(xfB.Q, f.axis)
		pointB := TransformPoint(xfB, f.localPoint)

		axisA := InvRotateVector(xfA.Q, Neg(normal))

		indexB := -1
		indexA := findSupport(f.proxyA, axisA)

		localPointA := f.proxyA.Points[indexA]
		pointA := TransformPoint(xfA, localPointA)

		separation := Dot(Sub(pointA, pointB), normal)
		return separation, indexA, indexB

	default:
		assert(false)
		return 0.0, -1, -1
	}
}

// evaluateSeparation is upstream b2EvaluateSeparation.
func evaluateSeparation(f *separationFunction, indexA, indexB int, t float64) float64 {
	xfA := GetSweepTransform(&f.sweepA, t)
	xfB := GetSweepTransform(&f.sweepB, t)

	switch f.typ {
	case pointsType:
		localPointA := f.proxyA.Points[indexA]
		localPointB := f.proxyB.Points[indexB]

		pointA := TransformPoint(xfA, localPointA)
		pointB := TransformPoint(xfB, localPointB)

		separation := Dot(Sub(pointB, pointA), f.axis)
		return separation

	case faceAType:
		normal := RotateVector(xfA.Q, f.axis)
		pointA := TransformPoint(xfA, f.localPoint)

		localPointB := f.proxyB.Points[indexB]
		pointB := TransformPoint(xfB, localPointB)

		separation := Dot(Sub(pointB, pointA), normal)
		return separation

	case faceBType:
		normal := RotateVector(xfB.Q, f.axis)
		pointB := TransformPoint(xfB, f.localPoint)

		localPointA := f.proxyA.Points[indexA]
		pointA := TransformPoint(xfA, localPointA)

		separation := Dot(Sub(pointA, pointB), normal)
		return separation

	default:
		assert(false)
		return 0.0
	}
}

// TimeOfImpact computes the upper bound on time before two shapes penetrate
// (upstream b2TimeOfImpact). Time is represented as a fraction between
// [0,tMax]. This uses a swept separating axis and may miss some intermediate,
// non-tunneling collisions. If you change the time interval, you should call
// this function again.
//
// CCD via the local separating axis method. This seeks progression by
// computing the largest time at which separation is maintained.
func TimeOfImpact(input *TOIInput) TOIOutput {
	var output TOIOutput
	output.State = TOIStateUnknown
	output.Fraction = input.MaxFraction

	sweepA := input.SweepA
	sweepB := input.SweepB
	assert(IsNormalizedRot(sweepA.Q1) && IsNormalizedRot(sweepA.Q2))
	assert(IsNormalizedRot(sweepB.Q1) && IsNormalizedRot(sweepB.Q2))

	// todo_erin
	// c1 can be at the origin yet the points are far away
	// b2Vec2 origin = b2Add(sweepA.c1, input->proxyA.points[0]);

	proxyA := &input.ProxyA
	proxyB := &input.ProxyB

	tMax := input.MaxFraction

	// Setup target distance and tolerance
	totalRadius := proxyA.Radius + proxyB.Radius
	target := maxFloat(LinearSlop, totalRadius-LinearSlop)
	tolerance := float64(0.25 * LinearSlop)
	assert(target > tolerance)

	t1 := 0.0
	const kMaxIterations = 20
	distanceIterations := 0

	// Prepare input for distance query.
	var cache SimplexCache
	var distanceInput DistanceInput
	distanceInput.ProxyA = input.ProxyA
	distanceInput.ProxyB = input.ProxyB
	distanceInput.UseRadii = false

	// The outer loop progressively attempts to compute new separating axes.
	// This loop terminates when an axis is repeated (no progress is made).
	for {
		// Get the distance between shapes. We can also use the results to get a
		// separating axis.
		distanceInput.TransformA = GetSweepTransform(&sweepA, t1)
		distanceInput.TransformB = GetSweepTransform(&sweepB, t1)
		distanceOutput := ShapeDistance(&distanceInput, &cache, nil)

		distanceIterations++

		// If the shapes are overlapped, we give up on continuous collision.
		if distanceOutput.Distance <= 0.0 {
			// Failure!
			output.State = TOIStateOverlapped
			output.Fraction = 0.0
			break
		}

		if distanceOutput.Distance <= target+tolerance {
			// Success!
			output.State = TOIStateHit
			// Averaged hit point
			pA := MulAdd(distanceOutput.PointA, proxyA.Radius, distanceOutput.Normal)
			pB := MulAdd(distanceOutput.PointB, -proxyB.Radius, distanceOutput.Normal)
			output.Point = Lerp(pA, pB, 0.5)
			output.Normal = distanceOutput.Normal
			output.Fraction = t1
			break
		}

		// Initialize the separating axis.
		fcn := makeSeparationFunction(cache, proxyA, &sweepA, proxyB, &sweepB, t1)

		// Compute the TOI on the separating axis. We do this by successively
		// resolving the deepest point. This loop is bounded by the number of
		// vertices.
		done := false
		t2 := tMax
		pushBackIterations := 0
		for {
			// Find the deepest point at t2. Store the witness point indices.
			s2, indexA, indexB := findMinSeparation(&fcn, t2)

			// Is the final configuration separated?
			if s2 > target+tolerance {
				// Victory!
				output.State = TOIStateSeparated
				output.Fraction = tMax
				done = true
				break
			}

			// Has the separation reached tolerance?
			if s2 > target-tolerance {
				// Advance the sweeps
				t1 = t2
				break
			}

			// Compute the initial separation of the witness points.
			s1 := evaluateSeparation(&fcn, indexA, indexB, t1)

			// Check for initial overlap. This might happen if the root finder
			// runs out of iterations.
			if s1 < target-tolerance {
				output.State = TOIStateFailed
				output.Fraction = t1
				done = true
				break
			}

			// Check for touching
			if s1 <= target+tolerance {
				// Success! t1 should hold the TOI (could be 0.0).
				output.State = TOIStateHit
				// Averaged hit point
				pA := MulAdd(distanceOutput.PointA, proxyA.Radius, distanceOutput.Normal)
				pB := MulAdd(distanceOutput.PointB, -proxyB.Radius, distanceOutput.Normal)
				output.Point = Lerp(pA, pB, 0.5)
				output.Normal = distanceOutput.Normal
				output.Fraction = t1
				done = true
				break
			}

			// Compute 1D root of: f(x) - target = 0
			rootIterationCount := 0
			a1, a2 := t1, t2
			for {
				// Use a mix of false position and bisection.
				var t float64
				if rootIterationCount&1 == 1 {
					// False position to improve convergence.
					// upstream: a1 + ( target - s1 ) * ( a2 - a1 ) / ( s2 - s1 )
					t = a1 + float64((target-s1)*(a2-a1))/(s2-s1)
				} else {
					// Bisection to guarantee progress.
					t = 0.5 * (a1 + a2)
				}

				rootIterationCount++

				s := evaluateSeparation(&fcn, indexA, indexB, t)

				// Has the separation reached tolerance?
				if absFloat(s-target) < tolerance {
					// t2 holds a tentative value for t1
					t2 = t
					break
				}

				// Ensure we continue to bracket the root.
				if s > target {
					a1 = t
					s1 = s
				} else {
					a2 = t
					s2 = s
				}

				if rootIterationCount == 50 {
					break
				}
			}

			pushBackIterations++

			if pushBackIterations == MaxPolygonVertices {
				break
			}
		}

		if done {
			break
		}

		if distanceIterations == kMaxIterations {
			// Root finder got stuck. Semi-victory.
			output.State = TOIStateFailed
			// Averaged hit point
			pA := MulAdd(distanceOutput.PointA, proxyA.Radius, distanceOutput.Normal)
			pB := MulAdd(distanceOutput.PointB, -proxyB.Radius, distanceOutput.Normal)
			output.Point = Lerp(pA, pB, 0.5)
			output.Normal = distanceOutput.Normal
			output.Fraction = t1
			break
		}
	}

	return output
}
