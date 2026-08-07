// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file include/box2d/collision.h.
// This port uses float64 where upstream uses float.
//
// Stage E2 ports the shape-geometry declarations and stage E3 adds the
// distance/GJK/TOI declarations. The manifold and dynamic-tree declarations
// from the same upstream header arrive with stage E4.

package box2d

// MaxPolygonVertices is the maximum number of vertices on a convex polygon
// (upstream B2_MAX_POLYGON_VERTICES). Changing this affects performance even
// if you don't use more vertices.
const MaxPolygonVertices = 8

// RayCastInput is low level ray cast input data.
type RayCastInput struct {
	// Origin is the start point of the ray cast.
	Origin Vec2

	// Translation is the translation of the ray cast.
	Translation Vec2

	// MaxFraction is the maximum fraction of the translation to consider,
	// typically 1.
	MaxFraction float64
}

// ShapeProxy is a distance proxy used by the GJK algorithm. It encapsulates
// any shape. You can provide between 1 and MaxPolygonVertices points and a
// radius.
type ShapeProxy struct {
	// Points is the point cloud.
	Points [MaxPolygonVertices]Vec2

	// Count is the number of points. Must be greater than 0.
	Count int

	// Radius is the external radius of the point cloud. May be zero.
	Radius float64
}

// ShapeCastInput is low level shape cast input in generic form. This allows
// casting an arbitrary point cloud wrap with a radius. For example, a circle
// is a single point with a non-zero radius. A capsule is two points with a
// non-zero radius. A box is four points with a zero radius.
type ShapeCastInput struct {
	// Proxy is a generic shape.
	Proxy ShapeProxy

	// Translation is the translation of the shape cast.
	Translation Vec2

	// MaxFraction is the maximum fraction of the translation to consider,
	// typically 1.
	MaxFraction float64

	// CanEncroach allows the shape cast to encroach when initially touching.
	// This only works if the radius is greater than zero.
	CanEncroach bool
}

// CastOutput is low level ray cast or shape-cast output data. It returns a
// zero fraction and normal in the case of initial overlap.
type CastOutput struct {
	// Normal is the surface normal at the hit point.
	Normal Vec2

	// Point is the surface hit point.
	Point Vec2

	// Fraction is the fraction of the input translation at collision.
	Fraction float64

	// Iterations is the number of iterations used.
	Iterations int

	// Hit reports whether the cast hit.
	Hit bool
}

// MassData holds the mass data computed for a shape.
type MassData struct {
	// Mass is the mass of the shape, usually in kilograms.
	Mass float64

	// Center is the position of the shape's centroid relative to the shape's
	// origin.
	Center Vec2

	// RotationalInertia is the rotational inertia of the shape about the shape
	// center.
	RotationalInertia float64
}

// Circle is a solid circle.
type Circle struct {
	// Center is the local center.
	Center Vec2

	// Radius is the radius.
	Radius float64
}

// Capsule is a solid capsule. It can be viewed as two semicircles connected by
// a rectangle.
type Capsule struct {
	// Center1 is the local center of the first semicircle.
	Center1 Vec2

	// Center2 is the local center of the second semicircle.
	Center2 Vec2

	// Radius is the radius of the semicircles.
	Radius float64
}

// Polygon is a solid convex polygon. It is assumed that the interior of the
// polygon is to the left of each edge. Polygons have a maximum number of
// vertices equal to MaxPolygonVertices. In most cases you should not need many
// vertices for a convex polygon.
//
// Do NOT fill this out manually, instead use a helper function like MakePolygon
// or MakeBox.
type Polygon struct {
	// Vertices are the polygon vertices.
	Vertices [MaxPolygonVertices]Vec2

	// Normals are the outward normal vectors of the polygon sides.
	Normals [MaxPolygonVertices]Vec2

	// Centroid is the centroid of the polygon.
	Centroid Vec2

	// Radius is the external radius for rounded polygons.
	Radius float64

	// Count is the number of polygon vertices.
	Count int
}

// Segment is a line segment with two-sided collision.
type Segment struct {
	// Point1 is the first point.
	Point1 Vec2

	// Point2 is the second point.
	Point2 Vec2
}

// ChainSegment is a line segment with one-sided collision. It only collides on
// the right side. Several of these are generated for a chain shape.
//
//	ghost1 -> point1 -> point2 -> ghost2
type ChainSegment struct {
	// Ghost1 is the tail ghost vertex.
	Ghost1 Vec2

	// Segment is the line segment.
	Segment Segment

	// Ghost2 is the head ghost vertex.
	Ghost2 Vec2

	// ChainID is the owning chain shape index (internal usage only).
	ChainID int
}

// Hull is a convex hull. It is used to create convex polygons.
//
// Do not modify these values directly, instead use ComputeHull.
type Hull struct {
	// Points are the final points of the hull.
	Points [MaxPolygonVertices]Vec2

	// Count is the number of points.
	Count int
}

// PlaneResult is a collision plane returned from World_CollideMover.
type PlaneResult struct {
	// Plane is the collision plane between the mover and a convex shape.
	Plane Plane

	// Point is the collision point on the shape.
	Point Vec2

	// Hit reports whether the collision registered a hit. If not, this plane
	// should be ignored.
	Hit bool
}

// SegmentDistanceResult holds the result of computing the distance between two
// line segments.
type SegmentDistanceResult struct {
	// Closest1 is the closest point on the first segment.
	Closest1 Vec2

	// Closest2 is the closest point on the second segment.
	Closest2 Vec2

	// Fraction1 is the barycentric coordinate on the first segment.
	Fraction1 float64

	// Fraction2 is the barycentric coordinate on the second segment.
	Fraction2 float64

	// DistanceSquared is the squared distance between the closest points.
	DistanceSquared float64
}

// SimplexCache is used to warm start the GJK simplex. If you call ShapeDistance
// multiple times with nearby transforms this might improve performance.
// Otherwise you can zero initialize this. The distance cache must be
// initialized to zero on the first call. Users should generally just zero
// initialize this structure for each call.
type SimplexCache struct {
	// Count is the number of stored simplex points.
	Count uint16

	// IndexA holds the cached simplex indices on shape A.
	IndexA [3]uint8

	// IndexB holds the cached simplex indices on shape B.
	IndexB [3]uint8
}

// DistanceInput is input for ShapeDistance.
type DistanceInput struct {
	// ProxyA is the proxy for shape A.
	ProxyA ShapeProxy

	// ProxyB is the proxy for shape B.
	ProxyB ShapeProxy

	// TransformA is the world transform for shape A.
	TransformA Transform

	// TransformB is the world transform for shape B.
	TransformB Transform

	// UseRadii selects whether the proxy radius is considered.
	UseRadii bool
}

// DistanceOutput is output for ShapeDistance.
type DistanceOutput struct {
	// PointA is the closest point on shapeA.
	PointA Vec2

	// PointB is the closest point on shapeB.
	PointB Vec2

	// Normal is the normal vector that points from A to B. Invalid if distance
	// is zero.
	Normal Vec2

	// Distance is the final distance, zero if overlapped.
	Distance float64

	// Iterations is the number of GJK iterations used.
	Iterations int

	// SimplexCount is the number of simplexes stored in the simplex array.
	SimplexCount int
}

// SimplexVertex is a simplex vertex for debugging the GJK algorithm.
type SimplexVertex struct {
	// WA is the support point in proxyA.
	WA Vec2

	// WB is the support point in proxyB.
	WB Vec2

	// W is wA - wB.
	W Vec2

	// A is the barycentric coordinate for the closest point.
	A float64

	// IndexA is the wA index.
	IndexA int

	// IndexB is the wB index.
	IndexB int
}

// Simplex is a simplex from the GJK algorithm.
type Simplex struct {
	// V1, V2, V3 are the vertices.
	V1, V2, V3 SimplexVertex

	// Count is the number of valid vertices.
	Count int
}

// ShapeCastPairInput holds input parameters for ShapeCast.
type ShapeCastPairInput struct {
	// ProxyA is the proxy for shape A.
	ProxyA ShapeProxy

	// ProxyB is the proxy for shape B.
	ProxyB ShapeProxy

	// TransformA is the world transform for shape A.
	TransformA Transform

	// TransformB is the world transform for shape B.
	TransformB Transform

	// TranslationB is the translation of shape B.
	TranslationB Vec2

	// MaxFraction is the fraction of the translation to consider, typically 1.
	MaxFraction float64

	// CanEncroach allows shapes with a radius to move slightly closer if
	// already touching.
	CanEncroach bool
}

// Sweep describes the motion of a body/shape for TOI computation. Shapes are
// defined with respect to the body origin, which may not coincide with the
// center of mass. However, to support dynamics we must interpolate the center
// of mass position.
type Sweep struct {
	// LocalCenter is the local center of mass position.
	LocalCenter Vec2

	// C1 is the starting center of mass world position.
	C1 Vec2

	// C2 is the ending center of mass world position.
	C2 Vec2

	// Q1 is the starting world rotation.
	Q1 Rot

	// Q2 is the ending world rotation.
	Q2 Rot
}

// TOIInput is time of impact input.
type TOIInput struct {
	// ProxyA is the proxy for shape A.
	ProxyA ShapeProxy

	// ProxyB is the proxy for shape B.
	ProxyB ShapeProxy

	// SweepA is the movement of shape A.
	SweepA Sweep

	// SweepB is the movement of shape B.
	SweepB Sweep

	// MaxFraction defines the sweep interval [0, maxFraction].
	MaxFraction float64
}

// TOIState describes the TOI output.
type TOIState int

// TOIState values (upstream b2TOIState enumerators).
const (
	TOIStateUnknown TOIState = iota
	TOIStateFailed
	TOIStateOverlapped
	TOIStateHit
	TOIStateSeparated
)

// TOIOutput is time of impact output.
type TOIOutput struct {
	// State is the type of result.
	State TOIState

	// Point is the hit point.
	Point Vec2

	// Normal is the hit normal.
	Normal Vec2

	// Fraction is the sweep time of the collision.
	Fraction float64
}
