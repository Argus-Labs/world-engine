// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/shape.c, src/shape.h.
//
// Public API mapping: b2Shape_GetDensity → (*World).ShapeDensity,
// b2Shape_SetDensity → (*World).SetShapeDensity, and so on (getters are
// Get-less). b2Shape_ComputeMassData → (*World).ShapeComputeMassData.
//
// Deviations from upstream:
//   - The geometry union on b2Shape becomes separate fields (Go has no
//     unions); only the field selected by shapeType is meaningful.
//   - b2Shape_GetWorld / b2Chain_GetWorld are not ported: there is no world
//     registry, callers already hold the *World (see world.go).
//   - The b2CollideMover* dispatch (b2CollideMover) is ported here; the
//     per-shape b2CollideMoverAnd* functions live in geometry.go.

package box2d

import "math"

// shape is the internal shape representation (upstream b2Shape).
type shape struct {
	id          int
	bodyID      int
	prevShapeID int
	nextShapeID int
	sensorIndex int
	shapeType   ShapeType
	material    SurfaceMaterial
	density     float64
	aabbMargin  float64
	aabb        AABB
	fatAABB     AABB

	localCentroid Vec2
	proxyKey      int

	filter Filter

	// User data. Deviation from upstream: the C void* becomes a uint64 so the
	// ECS wrapper can pack an entity id directly.
	userData uint64

	// Upstream union: only the member selected by shapeType is meaningful.
	capsule      Capsule
	circle       Circle
	polygon      Polygon
	segment      Segment
	chainSegment ChainSegment

	generation            uint16
	enableSensorEvents    bool
	enableContactEvents   bool
	enableCustomFiltering bool
	enableHitEvents       bool
	enablePreSolveEvents  bool
	enlargedAABB          bool
}

// chainShape is the internal chain shape representation
// (upstream b2ChainShape).
//
// Deviation from upstream: the pointer+count pairs (shapeIndices/count and
// materials/materialCount) become slices; use len to recover the counts.
type chainShape struct {
	id          int
	bodyID      int
	nextChainID int

	shapeIndices []int
	materials    []SurfaceMaterial
	generation   uint16
}

// shapeExtent holds min/max extents of a shape relative to a local center
// (upstream b2ShapeExtent).
type shapeExtent struct {
	minExtent float64
	maxExtent float64
}

// getShape returns a validated shape (upstream static b2GetShape).
func (w *World) getShape(shapeID ShapeID) *shape {
	id := int(shapeID.index1) - 1
	s := &w.shapes[id]
	assert(w.ownsToken(shapeID.world0) && s.id == id && s.generation == shapeID.generation)
	return s
}

// getChainShape returns a validated chain shape (upstream static
// b2GetChainShape).
func (w *World) getChainShape(chainID ChainID) *chainShape {
	id := int(chainID.index1) - 1
	chain := &w.chainShapes[id]
	assert(w.ownsToken(chainID.world0) && chain.id == id && chain.generation == chainID.generation)
	return chain
}

// computeShapeMargin computes the fat AABB margin for a shape (upstream
// static b2ComputeShapeMargin).
func computeShapeMargin(s *shape) float64 {
	var margin float64

	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		margin = mulAdd(0.5, Distance(s.capsule.Center2, s.capsule.Center1), s.capsule.Radius)

	case CircleShape:
		margin = s.circle.Radius

	case PolygonShape:
		poly := &s.polygon
		maxExtentSqr := 0.0
		count := poly.Count
		for i := range count {
			distanceSqr := DistanceSquared(poly.Vertices[i], poly.Centroid)
			maxExtentSqr = maxFloat(maxExtentSqr, distanceSqr)
		}

		margin = math.Sqrt(maxExtentSqr)

	case SegmentShape:
		margin = 0.5 * Distance(s.segment.Point1, s.segment.Point2)

	case ChainSegmentShape:
		margin = 0.5 * Distance(s.chainSegment.Segment.Point1, s.chainSegment.Segment.Point2)

	default:
		return MaxAABBMargin
	}

	return minFloat(MaxAABBMargin, AABBMarginFraction*margin)
}

// updateShapeAABBs recomputes the tight and fat AABBs of a shape (upstream
// static b2UpdateShapeAABBs).
func updateShapeAABBs(s *shape, transform Transform, proxyType BodyType) {
	// Compute a bounding box with a speculative margin
	speculativeDistance := SpeculativeDistance
	aabbMargin := s.aabbMargin

	aabb := computeShapeAABB(s, transform)
	aabb.LowerBound.X -= speculativeDistance
	aabb.LowerBound.Y -= speculativeDistance
	aabb.UpperBound.X += speculativeDistance
	aabb.UpperBound.Y += speculativeDistance
	s.aabb = aabb

	// Smaller margin for static bodies. Cannot be zero due to TOI tolerance.
	margin := aabbMargin
	if proxyType == StaticBody {
		margin = speculativeDistance
	}
	var fatAABB AABB
	fatAABB.LowerBound.X = aabb.LowerBound.X - margin
	fatAABB.LowerBound.Y = aabb.LowerBound.Y - margin
	fatAABB.UpperBound.X = aabb.UpperBound.X + margin
	fatAABB.UpperBound.Y = aabb.UpperBound.Y + margin
	s.fatAABB = fatAABB
}

// createShapeInternal allocates a shape slot, fills it, creates the
// broad-phase proxy and links the shape into the body (upstream static
// b2CreateShapeInternal).
func (w *World) createShapeInternal(b *body, transform Transform, def *ShapeDef,
	geometry any, shapeType ShapeType,
) *shape {
	shapeID := allocID(&w.shapeIDPool)

	if shapeID == len(w.shapes) {
		w.shapes = append(w.shapes, shape{})
	} else {
		assert(w.shapes[shapeID].id == NullIndex)
	}

	s := &w.shapes[shapeID]

	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch shapeType {
	case CapsuleShape:
		s.capsule = *geometry.(*Capsule) //nolint:errcheck // a mistyped geometry is a programmer error; the panic mirrors the upstream void* cast

	case CircleShape:
		s.circle = *geometry.(*Circle) //nolint:errcheck // a mistyped geometry is a programmer error; the panic mirrors the upstream void* cast

	case PolygonShape:
		s.polygon = *geometry.(*Polygon) //nolint:errcheck // a mistyped geometry is a programmer error; the panic mirrors the upstream void* cast

	case SegmentShape:
		s.segment = *geometry.(*Segment) //nolint:errcheck // a mistyped geometry is a programmer error; the panic mirrors the upstream void* cast

	case ChainSegmentShape:
		s.chainSegment = *geometry.(*ChainSegment) //nolint:errcheck // a mistyped geometry is a programmer error; the panic mirrors the upstream void* cast

	default:
		assert(false)
	}

	s.id = shapeID
	s.bodyID = b.id
	s.shapeType = shapeType
	s.density = def.Density
	s.material = def.Material
	s.filter = def.Filter
	s.userData = def.UserData
	s.enlargedAABB = false
	s.enableSensorEvents = def.EnableSensorEvents
	s.enableContactEvents = def.EnableContactEvents
	s.enableCustomFiltering = def.EnableCustomFiltering
	s.enableHitEvents = def.EnableHitEvents
	s.enablePreSolveEvents = def.EnablePreSolveEvents
	s.proxyKey = NullIndex
	s.localCentroid = getShapeCentroid(s)
	s.aabbMargin = computeShapeMargin(s)
	s.aabb = AABB{LowerBound: Vec2Zero, UpperBound: Vec2Zero}
	s.fatAABB = AABB{LowerBound: Vec2Zero, UpperBound: Vec2Zero}
	s.generation++

	if b.setIndex != disabledSet {
		proxyType := b.bodyType
		createShapeProxy(s, &w.broadPhase, proxyType, transform, def.InvokeContactCreation || def.IsSensor)
	}

	// Add to shape doubly linked list
	if b.headShapeID != NullIndex {
		headShape := &w.shapes[b.headShapeID]
		headShape.prevShapeID = shapeID
	}

	s.prevShapeID = NullIndex
	s.nextShapeID = b.headShapeID
	b.headShapeID = shapeID
	b.shapeCount++

	if def.IsSensor {
		s.sensorIndex = len(w.sensors)
		w.sensors = append(w.sensors, sensor{
			hits:      make([]visitor, 0, 4),
			overlaps1: make([]visitor, 0, 16),
			overlaps2: make([]visitor, 0, 16),
			shapeID:   shapeID,
		})
	} else {
		s.sensorIndex = NullIndex
	}

	w.validateSolverSets()

	return s
}

// createShape validates the definition and creates a shape on a body
// (upstream static b2CreateShape).
func (w *World) createShape(bodyID BodyID, def *ShapeDef, geometry any, shapeType ShapeType) ShapeID {
	requireInitialized(def.initialized, "ShapeDef", "DefaultShapeDef")
	requireValidDefField(IsValidFloat(def.Density), "ShapeDef", "Density",
		"must be a finite number")
	requireValidDefField(IsValidFloat(def.Material.Friction), "ShapeDef", "Material.Friction",
		"must be a finite number")
	requireValidDefField(IsValidFloat(def.Material.Restitution), "ShapeDef", "Material.Restitution",
		"must be a finite number")

	assert(def.Density >= 0.0)
	assert(def.Material.Friction >= 0.0)
	assert(def.Material.Restitution >= 0.0)
	assert(IsValidFloat(def.Material.RollingResistance) && def.Material.RollingResistance >= 0.0)
	assert(IsValidFloat(def.Material.TangentSpeed))

	if w.locked {
		assert(false)
		return ShapeID{}
	}

	b := w.getBodyFullID(bodyID)
	transform := w.getBodyTransformQuick(b)

	s := w.createShapeInternal(b, transform, def, geometry, shapeType)

	if def.UpdateBodyMass {
		w.updateBodyMassData(b)
	} else {
		b.flags |= dirtyMass
	}

	w.validateSolverSets()

	return ShapeID{index1: int32(s.id + 1), world0: bodyID.world0, generation: s.generation}
}

// CreateCircleShape creates a circle shape and attaches it to a body. The
// shape definition and geometry are fully cloned. Contacts are not created
// until the next time step (upstream b2CreateCircleShape).
func (w *World) CreateCircleShape(bodyID BodyID, def *ShapeDef, circle *Circle) ShapeID {
	return w.createShape(bodyID, def, circle, CircleShape)
}

// CreateCapsuleShape creates a capsule shape and attaches it to a body. The
// shape definition and geometry are fully cloned. Contacts are not created
// until the next time step (upstream b2CreateCapsuleShape).
func (w *World) CreateCapsuleShape(bodyID BodyID, def *ShapeDef, capsule *Capsule) ShapeID {
	lengthSqr := DistanceSquared(capsule.Center1, capsule.Center2)
	if lengthSqr <= LinearSlop*LinearSlop {
		return ShapeID{}
	}

	return w.createShape(bodyID, def, capsule, CapsuleShape)
}

// CreatePolygonShape creates a polygon shape and attaches it to a body. The
// shape definition and geometry are fully cloned. Contacts are not created
// until the next time step (upstream b2CreatePolygonShape).
func (w *World) CreatePolygonShape(bodyID BodyID, def *ShapeDef, polygon *Polygon) ShapeID {
	// Public API precondition: the polygon is cloned into the world and then
	// indexed by Count from the collision, mass and debug-draw code every step.
	// Reject a hand-built Polygon here rather than panicking inside a later
	// World.Step. MakePolygon and MakeBox already guarantee this range;
	// ComputeHull/ValidateHull only constrain the hull, not a Polygon literal.
	requireValidPolygonShapeCount(polygon)

	assert(IsValidFloat(polygon.Radius) && polygon.Radius >= 0.0)
	return w.createShape(bodyID, def, polygon, PolygonShape)
}

// CreateSegmentShape creates a line segment shape and attaches it to a body.
// The shape definition and geometry are fully cloned. Contacts are not
// created until the next time step (upstream b2CreateSegmentShape).
func (w *World) CreateSegmentShape(bodyID BodyID, def *ShapeDef, segment *Segment) ShapeID {
	lengthSqr := DistanceSquared(segment.Point1, segment.Point2)
	if lengthSqr <= LinearSlop*LinearSlop {
		assert(false)
		return ShapeID{}
	}

	return w.createShape(bodyID, def, segment, SegmentShape)
}

// destroyShapeInternal destroys a shape on a body. This doesn't need to be
// called when destroying a body (upstream static b2DestroyShapeInternal).
//

func (w *World) destroyShapeInternal(s *shape, b *body, wakeBodies bool) {
	shapeID := s.id

	// Remove the shape from the body's doubly linked list.
	if s.prevShapeID != NullIndex {
		prevShape := &w.shapes[s.prevShapeID]
		prevShape.nextShapeID = s.nextShapeID
	}

	if s.nextShapeID != NullIndex {
		nextShape := &w.shapes[s.nextShapeID]
		nextShape.prevShapeID = s.prevShapeID
	}

	if shapeID == b.headShapeID {
		b.headShapeID = s.nextShapeID
	}

	b.shapeCount--

	// Remove from broad-phase.
	destroyShapeProxy(s, &w.broadPhase)

	// Destroy any contacts associated with the shape.
	contactKey := b.headContactKey
	for contactKey != NullIndex {
		contactID := contactKey >> 1
		edgeIndex := contactKey & 1

		c := &w.contacts[contactID]
		contactKey = c.edges[edgeIndex].nextKey

		if c.shapeIDA == shapeID || c.shapeIDB == shapeID {
			w.destroyContact(c, wakeBodies)
		}
	}

	if s.sensorIndex != NullIndex {
		sen := &w.sensors[s.sensorIndex]
		for i := range sen.overlaps2 {
			ref := &sen.overlaps2[i]
			event := SensorEndTouchEvent{
				SensorShapeID: ShapeID{
					index1:     int32(shapeID + 1),
					world0:     w.worldID,
					generation: s.generation,
				},
				VisitorShapeID: ShapeID{
					index1:     int32(ref.shapeID + 1),
					world0:     w.worldID,
					generation: ref.generation,
				},
			}

			w.sensorEndEvents[w.endEventArrayIndex] = append(w.sensorEndEvents[w.endEventArrayIndex], event)
		}

		// Destroy sensor
		sen.hits = nil
		sen.overlaps1 = nil
		sen.overlaps2 = nil

		movedIndex := removeSwap(&w.sensors, s.sensorIndex)
		if movedIndex != NullIndex {
			// Fixup moved sensor
			movedSensor := &w.sensors[s.sensorIndex]
			otherSensorShape := &w.shapes[movedSensor.shapeID]
			otherSensorShape.sensorIndex = s.sensorIndex
		}
	}

	// Return shape to free list.
	freeID(&w.shapeIDPool, shapeID)
	s.id = NullIndex

	w.validateSolverSets()
}

// DestroyShape destroys a shape. You may defer the body mass update which can
// improve performance if several shapes on a body are destroyed at once
// (upstream b2DestroyShape).
func (w *World) DestroyShape(shapeID ShapeID, updateBodyMass bool) {
	if w.locked {
		assert(false)
		return
	}

	// Reject an id minted by another world (see DestroyBody).
	if !w.ownsToken(shapeID.world0) {
		assert(false)
		return
	}

	s := w.getShape(shapeID)

	// need to wake bodies because this might be a static body
	wakeBodies := true
	b := &w.bodies[s.bodyID]
	w.destroyShapeInternal(s, b, wakeBodies)

	if updateBodyMass {
		w.updateBodyMassData(b)
	}
}

// CreateChain creates a chain shape (upstream b2CreateChain).
func (w *World) CreateChain(bodyID BodyID, def *ChainDef) ChainID {
	requireInitialized(def.initialized, "ChainDef", "DefaultChainDef")
	assert(len(def.Points) >= 4)
	assert(len(def.Materials) == 1 || len(def.Materials) == len(def.Points))

	if w.locked {
		assert(false)
		return ChainID{}
	}

	b := w.getBodyFullID(bodyID)
	transform := w.getBodyTransformQuick(b)

	chainID := allocID(&w.chainIDPool)

	if chainID == len(w.chainShapes) {
		w.chainShapes = append(w.chainShapes, chainShape{})
	} else {
		assert(w.chainShapes[chainID].id == NullIndex)
	}

	chain := &w.chainShapes[chainID]

	chain.id = chainID
	chain.bodyID = b.id
	chain.nextChainID = b.headChainID
	chain.generation++

	materialCount := len(def.Materials)
	chain.materials = make([]SurfaceMaterial, materialCount)

	for i := range materialCount {
		material := &def.Materials[i]
		assert(IsValidFloat(material.Friction) && material.Friction >= 0.0)
		assert(IsValidFloat(material.Restitution) && material.Restitution >= 0.0)
		assert(IsValidFloat(material.RollingResistance) && material.RollingResistance >= 0.0)
		assert(IsValidFloat(material.TangentSpeed))

		chain.materials[i] = *material
	}

	b.headChainID = chainID

	shapeDef := DefaultShapeDef()
	shapeDef.UserData = def.UserData
	shapeDef.Filter = def.Filter
	shapeDef.EnableSensorEvents = def.EnableSensorEvents
	shapeDef.EnableContactEvents = false
	shapeDef.EnableHitEvents = false

	points := def.Points
	n := len(points)

	if def.IsLoop {
		chain.shapeIndices = make([]int, n)

		var cs ChainSegment

		prevIndex := n - 1
		for i := range n - 2 {
			cs.Ghost1 = points[prevIndex]
			cs.Segment.Point1 = points[i]
			cs.Segment.Point2 = points[i+1]
			cs.Ghost2 = points[i+2]
			cs.ChainID = chainID
			prevIndex = i

			materialIndex := 0
			if materialCount != 1 {
				materialIndex = i
			}
			shapeDef.Material = def.Materials[materialIndex]

			s := w.createShapeInternal(b, transform, &shapeDef, &cs, ChainSegmentShape)
			chain.shapeIndices[i] = s.id
		}

		{
			cs.Ghost1 = points[n-3]
			cs.Segment.Point1 = points[n-2]
			cs.Segment.Point2 = points[n-1]
			cs.Ghost2 = points[0]
			cs.ChainID = chainID

			materialIndex := 0
			if materialCount != 1 {
				materialIndex = n - 2
			}
			shapeDef.Material = def.Materials[materialIndex]

			s := w.createShapeInternal(b, transform, &shapeDef, &cs, ChainSegmentShape)
			chain.shapeIndices[n-2] = s.id
		}

		{
			cs.Ghost1 = points[n-2]
			cs.Segment.Point1 = points[n-1]
			cs.Segment.Point2 = points[0]
			cs.Ghost2 = points[1]
			cs.ChainID = chainID

			materialIndex := 0
			if materialCount != 1 {
				materialIndex = n - 1
			}
			shapeDef.Material = def.Materials[materialIndex]

			s := w.createShapeInternal(b, transform, &shapeDef, &cs, ChainSegmentShape)
			chain.shapeIndices[n-1] = s.id
		}
	} else {
		chain.shapeIndices = make([]int, n-3)

		var cs ChainSegment

		for i := range n - 3 {
			cs.Ghost1 = points[i]
			cs.Segment.Point1 = points[i+1]
			cs.Segment.Point2 = points[i+2]
			cs.Ghost2 = points[i+3]
			cs.ChainID = chainID

			// Material is associated with leading point of solid segment
			materialIndex := 0
			if materialCount != 1 {
				materialIndex = i + 1
			}
			shapeDef.Material = def.Materials[materialIndex]

			s := w.createShapeInternal(b, transform, &shapeDef, &cs, ChainSegmentShape)
			chain.shapeIndices[i] = s.id
		}
	}

	return ChainID{index1: int32(chainID + 1), world0: w.worldID, generation: chain.generation}
}

// freeChainData releases the shape index and material arrays of a chain
// (upstream b2FreeChainData).
func freeChainData(chain *chainShape) {
	chain.shapeIndices = nil
	chain.materials = nil
}

// DestroyChain destroys a chain shape (upstream b2DestroyChain).
func (w *World) DestroyChain(chainID ChainID) {
	if w.locked {
		assert(false)
		return
	}

	// Reject an id minted by another world (see DestroyBody).
	if !w.ownsToken(chainID.world0) {
		assert(false)
		return
	}

	chain := w.getChainShape(chainID)

	b := &w.bodies[chain.bodyID]

	// Remove the chain from the body's singly linked list.
	chainIDPtr := &b.headChainID
	found := false
	for *chainIDPtr != NullIndex {
		if *chainIDPtr == chain.id {
			*chainIDPtr = chain.nextChainID
			found = true
			break
		}

		chainIDPtr = &w.chainShapes[*chainIDPtr].nextChainID
	}

	assert(found)
	if !found {
		return
	}

	count := len(chain.shapeIndices)
	for i := range count {
		shapeID := chain.shapeIndices[i]
		s := &w.shapes[shapeID]
		wakeBodies := true
		w.destroyShapeInternal(s, b, wakeBodies)
	}

	freeChainData(chain)

	// Return chain to free list.
	freeID(&w.chainIDPool, chain.id)
	chain.id = NullIndex

	w.validateSolverSets()
}

// ChainSegmentCount returns the number of segments on this chain shape
// (upstream b2Chain_GetSegmentCount).
func (w *World) ChainSegmentCount(chainID ChainID) int {
	w.panicIfPoisoned()
	if w.locked {
		assert(false)
		return 0
	}

	chain := w.getChainShape(chainID)
	return len(chain.shapeIndices)
}

// ChainSegments fills segmentArray with the shape ids of the chain segments,
// up to len(segmentArray), and returns the number of ids stored
// (upstream b2Chain_GetSegments).
func (w *World) ChainSegments(chainID ChainID, segmentArray []ShapeID) int {
	w.panicIfPoisoned()
	if w.locked {
		assert(false)
		return 0
	}

	chain := w.getChainShape(chainID)

	count := minInt(len(chain.shapeIndices), len(segmentArray))
	for i := range count {
		shapeID := chain.shapeIndices[i]
		s := &w.shapes[shapeID]
		segmentArray[i] = ShapeID{index1: int32(shapeID + 1), world0: chainID.world0, generation: s.generation}
	}

	return count
}

// computeShapeAABB computes the AABB of a shape under a transform (upstream
// b2ComputeShapeAABB).
func computeShapeAABB(s *shape, xf Transform) AABB {
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		return ComputeCapsuleAABB(&s.capsule, xf)
	case CircleShape:
		return ComputeCircleAABB(&s.circle, xf)
	case PolygonShape:
		return ComputePolygonAABB(&s.polygon, xf)
	case SegmentShape:
		return ComputeSegmentAABB(&s.segment, xf)
	case ChainSegmentShape:
		return ComputeSegmentAABB(&s.chainSegment.Segment, xf)
	default:
		assert(false)
		return AABB{LowerBound: xf.P, UpperBound: xf.P}
	}
}

// getShapeCentroid returns the local centroid of a shape (upstream
// b2GetShapeCentroid).
func getShapeCentroid(s *shape) Vec2 {
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		return Lerp(s.capsule.Center1, s.capsule.Center2, 0.5)
	case CircleShape:
		return s.circle.Center
	case PolygonShape:
		return s.polygon.Centroid
	case SegmentShape:
		return Lerp(s.segment.Point1, s.segment.Point2, 0.5)
	case ChainSegmentShape:
		return Lerp(s.chainSegment.Segment.Point1, s.chainSegment.Segment.Point2, 0.5)
	default:
		return Vec2Zero
	}
}

// getShapePerimeter returns the perimeter of a shape, used by the explosion
// feature (upstream b2GetShapePerimeter).
func getShapePerimeter(s *shape) float64 {
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		return dot2(2.0, Length(Sub(s.capsule.Center1, s.capsule.Center2)), 2.0*Pi, s.capsule.Radius)
	case CircleShape:
		return 2.0 * Pi * s.circle.Radius
	case PolygonShape:
		points := s.polygon.Vertices
		count := s.polygon.Count
		perimeter := 2.0 * Pi * s.polygon.Radius
		assert(count > 0)
		prev := points[count-1]
		for i := range count {
			next := points[i]
			perimeter += Length(Sub(next, prev))
			prev = next
		}

		return perimeter
	case SegmentShape:
		return 2.0 * Length(Sub(s.segment.Point1, s.segment.Point2))
	case ChainSegmentShape:
		return 2.0 * Length(Sub(s.chainSegment.Segment.Point1, s.chainSegment.Segment.Point2))
	default:
		return 0.0
	}
}

// getShapeProjectedPerimeter projects the shape perimeter onto an infinite
// line (upstream b2GetShapeProjectedPerimeter).
func getShapeProjectedPerimeter(s *shape, line Vec2) float64 {
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		axis := Sub(s.capsule.Center2, s.capsule.Center1)
		projectedLength := absFloat(Dot(axis, line))
		return mulAdd(2.0, s.capsule.Radius, projectedLength)

	case CircleShape:
		return 2.0 * s.circle.Radius

	case PolygonShape:
		points := s.polygon.Vertices
		count := s.polygon.Count
		assert(count > 0)
		value := Dot(points[0], line)
		lower := value
		upper := value
		for i := 1; i < count; i++ {
			value = Dot(points[i], line)
			lower = minFloat(lower, value)
			upper = maxFloat(upper, value)
		}

		return mulAdd(2.0, s.polygon.Radius, upper-lower)

	case SegmentShape:
		value1 := Dot(s.segment.Point1, line)
		value2 := Dot(s.segment.Point2, line)
		return absFloat(value2 - value1)

	case ChainSegmentShape:
		value1 := Dot(s.chainSegment.Segment.Point1, line)
		value2 := Dot(s.chainSegment.Segment.Point2, line)
		return absFloat(value2 - value1)

	default:
		return 0.0
	}
}

// computeShapeMass computes the mass data of a shape from its geometry and
// density (upstream b2ComputeShapeMass).
func computeShapeMass(s *shape) MassData {
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		return ComputeCapsuleMass(&s.capsule, s.density)
	case CircleShape:
		return ComputeCircleMass(&s.circle, s.density)
	case PolygonShape:
		return ComputePolygonMass(&s.polygon, s.density)
	default:
		return MassData{}
	}
}

// computeShapeExtent computes the min/max extents of a shape relative to a
// local center (upstream b2ComputeShapeExtent).
func computeShapeExtent(s *shape, localCenter Vec2) shapeExtent {
	extent := shapeExtent{}

	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		radius := s.capsule.Radius
		extent.minExtent = radius
		c1 := Sub(s.capsule.Center1, localCenter)
		c2 := Sub(s.capsule.Center2, localCenter)
		extent.maxExtent = math.Sqrt(maxFloat(LengthSquared(c1), LengthSquared(c2))) + radius

	case CircleShape:
		radius := s.circle.Radius
		extent.minExtent = radius
		extent.maxExtent = Length(Sub(s.circle.Center, localCenter)) + radius

	case PolygonShape:
		poly := &s.polygon
		minExtent := Huge
		maxExtentSqr := 0.0
		count := poly.Count
		for i := range count {
			v := poly.Vertices[i]
			planeOffset := Dot(poly.Normals[i], Sub(v, poly.Centroid))
			minExtent = minFloat(minExtent, planeOffset)

			distanceSqr := LengthSquared(Sub(v, localCenter))
			maxExtentSqr = maxFloat(maxExtentSqr, distanceSqr)
		}

		extent.minExtent = minExtent + poly.Radius
		extent.maxExtent = math.Sqrt(maxExtentSqr) + poly.Radius

	case SegmentShape:
		extent.minExtent = 0.0
		c1 := Sub(s.segment.Point1, localCenter)
		c2 := Sub(s.segment.Point2, localCenter)
		extent.maxExtent = math.Sqrt(maxFloat(LengthSquared(c1), LengthSquared(c2)))

	case ChainSegmentShape:
		extent.minExtent = 0.0
		c1 := Sub(s.chainSegment.Segment.Point1, localCenter)
		c2 := Sub(s.chainSegment.Segment.Point2, localCenter)
		extent.maxExtent = math.Sqrt(maxFloat(LengthSquared(c1), LengthSquared(c2)))

	default:
	}

	return extent
}

// rayCastShape casts a ray against a shape in local space and converts the
// result to world space (upstream b2RayCastShape).
func rayCastShape(input *RayCastInput, s *shape, transform Transform) CastOutput {
	localInput := *input
	localInput.Origin = InvTransformPoint(transform, input.Origin)
	localInput.Translation = InvRotateVector(transform.Q, input.Translation)

	output := CastOutput{}
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		output = RayCastCapsule(&s.capsule, &localInput)
	case CircleShape:
		output = RayCastCircle(&s.circle, &localInput)
	case PolygonShape:
		output = RayCastPolygon(&s.polygon, &localInput)
	case SegmentShape:
		output = RayCastSegment(&s.segment, &localInput, false)
	case ChainSegmentShape:
		output = RayCastSegment(&s.chainSegment.Segment, &localInput, true)
	default:
		return output
	}

	output.Point = TransformPoint(transform, output.Point)
	output.Normal = RotateVector(transform.Q, output.Normal)
	return output
}

// shapeCastShape casts a shape proxy against a shape in local space and
// converts the result to world space (upstream b2ShapeCastShape).
func shapeCastShape(input *ShapeCastInput, s *shape, transform Transform) CastOutput {
	output := CastOutput{}

	if input.Proxy.Count == 0 {
		return output
	}

	localInput := *input

	for i := range localInput.Proxy.Count {
		localInput.Proxy.Points[i] = InvTransformPoint(transform, input.Proxy.Points[i])
	}

	localInput.Translation = InvRotateVector(transform.Q, input.Translation)

	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		output = ShapeCastCapsule(&s.capsule, &localInput)
	case CircleShape:
		output = ShapeCastCircle(&s.circle, &localInput)
	case PolygonShape:
		output = ShapeCastPolygon(&s.polygon, &localInput)
	case SegmentShape:
		output = ShapeCastSegment(&s.segment, &localInput)
	case ChainSegmentShape:
		// Check for back side collision
		approximateCentroid := localInput.Proxy.Points[0]
		for i := 1; i < localInput.Proxy.Count; i++ {
			approximateCentroid = Add(approximateCentroid, localInput.Proxy.Points[i])
		}

		approximateCentroid = MulSV(1.0/float64(localInput.Proxy.Count), approximateCentroid)

		edge := Sub(s.chainSegment.Segment.Point2, s.chainSegment.Segment.Point1)
		r := Sub(approximateCentroid, s.chainSegment.Segment.Point1)

		if Cross(r, edge) < 0.0 {
			// Shape cast starts behind
			return output
		}

		output = ShapeCastSegment(&s.chainSegment.Segment, &localInput)
	default:
		return output
	}

	output.Point = TransformPoint(transform, output.Point)
	output.Normal = RotateVector(transform.Q, output.Normal)
	return output
}

// collideMover collides a capsule mover with a shape (upstream
// b2CollideMover).
func collideMover(mover *Capsule, s *shape, transform Transform) PlaneResult {
	localMover := Capsule{
		Center1: InvTransformPoint(transform, mover.Center1),
		Center2: InvTransformPoint(transform, mover.Center2),
		Radius:  mover.Radius,
	}

	result := PlaneResult{}
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		result = CollideMoverAndCapsule(&localMover, &s.capsule)
	case CircleShape:
		result = CollideMoverAndCircle(&localMover, &s.circle)
	case PolygonShape:
		result = CollideMoverAndPolygon(&localMover, &s.polygon)
	case SegmentShape:
		result = CollideMoverAndSegment(&localMover, &s.segment)
	case ChainSegmentShape:
		result = CollideMoverAndSegment(&localMover, &s.chainSegment.Segment)
	default:
		return result
	}

	if !result.Hit {
		return result
	}

	result.Plane.Normal = RotateVector(transform.Q, result.Plane.Normal)
	return result
}

// createShapeProxy creates the broad-phase proxy for a shape (upstream
// b2CreateShapeProxy).
func createShapeProxy(s *shape, bp *broadPhase, proxyType BodyType, transform Transform, forcePairCreation bool) {
	assert(s.proxyKey == NullIndex)

	updateShapeAABBs(s, transform, proxyType)

	// Create proxies in the broad-phase.
	s.proxyKey = bp.createProxy(proxyType, s.fatAABB, s.filter.CategoryBits, s.id, forcePairCreation)
	assert(proxyKeyType(s.proxyKey) < BodyTypeCount)
}

// destroyShapeProxy destroys the broad-phase proxy of a shape (upstream
// b2DestroyShapeProxy).
func destroyShapeProxy(s *shape, bp *broadPhase) {
	if s.proxyKey != NullIndex {
		bp.destroyProxy(s.proxyKey)
		s.proxyKey = NullIndex
	}
}

// makeShapeDistanceProxy builds a GJK distance proxy from a shape (upstream
// b2MakeShapeDistanceProxy).
func makeShapeDistanceProxy(s *shape) ShapeProxy {
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		return MakeProxy([]Vec2{s.capsule.Center1, s.capsule.Center2}, 2, s.capsule.Radius)
	case CircleShape:
		return MakeProxy([]Vec2{s.circle.Center}, 1, s.circle.Radius)
	case PolygonShape:
		return MakeProxy(s.polygon.Vertices[:], s.polygon.Count, s.polygon.Radius)
	case SegmentShape:
		return MakeProxy([]Vec2{s.segment.Point1, s.segment.Point2}, 2, 0.0)
	case ChainSegmentShape:
		return MakeProxy([]Vec2{s.chainSegment.Segment.Point1, s.chainSegment.Segment.Point2}, 2, 0.0)
	default:
		assert(false)
		return ShapeProxy{}
	}
}

// ShapeBody returns the id of the body that a shape is attached to
// (upstream b2Shape_GetBody).
func (w *World) ShapeBody(shapeID ShapeID) BodyID {
	s := w.getShape(shapeID)
	return w.makeBodyID(s.bodyID)
}

// SetShapeUserData sets the user data on a shape
// (upstream b2Shape_SetUserData).
func (w *World) SetShapeUserData(shapeID ShapeID, userData uint64) {
	s := w.getShape(shapeID)
	s.userData = userData
}

// ShapeUserData returns the user data stored on a shape
// (upstream b2Shape_GetUserData).
func (w *World) ShapeUserData(shapeID ShapeID) uint64 {
	s := w.getShape(shapeID)
	return s.userData
}

// IsShapeSensor reports whether a shape is a sensor
// (upstream b2Shape_IsSensor).
func (w *World) IsShapeSensor(shapeID ShapeID) bool {
	s := w.getShape(shapeID)
	return s.sensorIndex != NullIndex
}

// ShapeTestPoint tests a point for overlap with a shape
// (upstream b2Shape_TestPoint).
func (w *World) ShapeTestPoint(shapeID ShapeID, point Vec2) bool {
	s := w.getShape(shapeID)

	transform := w.getBodyTransform(s.bodyID)
	localPoint := InvTransformPoint(transform, point)

	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		return PointInCapsule(&s.capsule, localPoint)

	case CircleShape:
		return PointInCircle(&s.circle, localPoint)

	case PolygonShape:
		return PointInPolygon(&s.polygon, localPoint)

	default:
		return false
	}
}

// ShapeRayCast casts a ray against a shape (upstream b2Shape_RayCast).
func (w *World) ShapeRayCast(shapeID ShapeID, input *RayCastInput) CastOutput {
	s := w.getShape(shapeID)

	transform := w.getBodyTransform(s.bodyID)

	// input in local coordinates
	var localInput RayCastInput
	localInput.Origin = InvTransformPoint(transform, input.Origin)
	localInput.Translation = InvRotateVector(transform.Q, input.Translation)
	localInput.MaxFraction = input.MaxFraction

	output := CastOutput{}
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		output = RayCastCapsule(&s.capsule, &localInput)

	case CircleShape:
		output = RayCastCircle(&s.circle, &localInput)

	case SegmentShape:
		output = RayCastSegment(&s.segment, &localInput, false)

	case PolygonShape:
		output = RayCastPolygon(&s.polygon, &localInput)

	case ChainSegmentShape:
		output = RayCastSegment(&s.chainSegment.Segment, &localInput, true)

	default:
		assert(false)
		return output
	}

	if output.Hit {
		// convert to world coordinates
		output.Normal = RotateVector(transform.Q, output.Normal)
		output.Point = TransformPoint(transform, output.Point)
	}

	return output
}

// SetShapeDensity sets the mass density of a shape, usually in kg/m^2. This
// will optionally update the mass properties on the parent body
// (upstream b2Shape_SetDensity).
func (w *World) SetShapeDensity(shapeID ShapeID, density float64, updateBodyMass bool) {
	assert(IsValidFloat(density) && density >= 0.0)

	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	if density == s.density {
		// early return to avoid expensive function
		return
	}

	s.density = density

	if updateBodyMass {
		b := &w.bodies[s.bodyID]
		w.updateBodyMassData(b)
	}
}

// ShapeDensity returns the density of a shape, usually in kg/m^2
// (upstream b2Shape_GetDensity).
func (w *World) ShapeDensity(shapeID ShapeID) float64 {
	s := w.getShape(shapeID)
	return s.density
}

// SetShapeFriction sets the friction on a shape
// (upstream b2Shape_SetFriction).
func (w *World) SetShapeFriction(shapeID ShapeID, friction float64) {
	assert(IsValidFloat(friction) && friction >= 0.0)

	assert(!w.locked)
	if w.locked {
		return
	}

	s := w.getShape(shapeID)
	s.material.Friction = friction
}

// ShapeFriction returns the friction of a shape
// (upstream b2Shape_GetFriction).
func (w *World) ShapeFriction(shapeID ShapeID) float64 {
	s := w.getShape(shapeID)
	return s.material.Friction
}

// SetShapeRestitution sets the shape restitution (bounciness)
// (upstream b2Shape_SetRestitution).
func (w *World) SetShapeRestitution(shapeID ShapeID, restitution float64) {
	assert(IsValidFloat(restitution) && restitution >= 0.0)

	assert(!w.locked)
	if w.locked {
		return
	}

	s := w.getShape(shapeID)
	s.material.Restitution = restitution
}

// ShapeRestitution returns the restitution of a shape
// (upstream b2Shape_GetRestitution).
func (w *World) ShapeRestitution(shapeID ShapeID) float64 {
	s := w.getShape(shapeID)
	return s.material.Restitution
}

// SetShapeUserMaterial sets the shape user material identifier
// (upstream b2Shape_SetUserMaterial).
func (w *World) SetShapeUserMaterial(shapeID ShapeID, material uint64) {
	assert(!w.locked)
	if w.locked {
		return
	}

	s := w.getShape(shapeID)
	s.material.UserMaterialID = material
}

// ShapeUserMaterial returns the shape user material identifier
// (upstream b2Shape_GetUserMaterial).
func (w *World) ShapeUserMaterial(shapeID ShapeID) uint64 {
	s := w.getShape(shapeID)
	return s.material.UserMaterialID
}

// ShapeSurfaceMaterial returns the shape surface material
// (upstream b2Shape_GetSurfaceMaterial).
func (w *World) ShapeSurfaceMaterial(shapeID ShapeID) SurfaceMaterial {
	s := w.getShape(shapeID)
	return s.material
}

// SetShapeSurfaceMaterial sets the shape surface material
// (upstream b2Shape_SetSurfaceMaterial).
func (w *World) SetShapeSurfaceMaterial(shapeID ShapeID, surfaceMaterial SurfaceMaterial) {
	s := w.getShape(shapeID)
	s.material = surfaceMaterial
}

// ShapeFilter returns the shape filter (upstream b2Shape_GetFilter).
func (w *World) ShapeFilter(shapeID ShapeID) Filter {
	s := w.getShape(shapeID)
	return s.filter
}

// resetProxy destroys contacts on a shape and refreshes or recreates its
// broad-phase proxy (upstream static b2ResetProxy).
//
//nolint:unparam // wakeBodies is always true, matching every upstream call site; keep the upstream signature.
func (w *World) resetProxy(s *shape, wakeBodies, destroyProxy bool) {
	b := &w.bodies[s.bodyID]

	shapeID := s.id

	// destroy all contacts associated with this shape
	contactKey := b.headContactKey
	for contactKey != NullIndex {
		contactID := contactKey >> 1
		edgeIndex := contactKey & 1

		c := &w.contacts[contactID]
		contactKey = c.edges[edgeIndex].nextKey

		if c.shapeIDA == shapeID || c.shapeIDB == shapeID {
			w.destroyContact(c, wakeBodies)
		}
	}

	transform := w.getBodyTransformQuick(b)
	if s.proxyKey != NullIndex {
		proxyType := proxyKeyType(s.proxyKey)
		updateShapeAABBs(s, transform, proxyType)

		if destroyProxy {
			w.broadPhase.destroyProxy(s.proxyKey)

			forcePairCreation := true
			s.proxyKey = w.broadPhase.createProxy(proxyType, s.fatAABB, s.filter.CategoryBits, shapeID,
				forcePairCreation)
		} else {
			w.broadPhase.moveProxy(s.proxyKey, s.fatAABB)
		}
	} else {
		proxyType := b.bodyType
		updateShapeAABBs(s, transform, proxyType)
	}

	w.validateSolverSets()
}

// SetShapeFilter sets the current filter. This is almost as expensive as
// recreating the shape. This may cause contacts to be immediately destroyed.
// However contacts are not created until the next world step. Sensor overlap
// state is also not updated until the next world step
// (upstream b2Shape_SetFilter).
func (w *World) SetShapeFilter(shapeID ShapeID, filter Filter) {
	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	if filter.MaskBits == s.filter.MaskBits && filter.CategoryBits == s.filter.CategoryBits &&
		filter.GroupIndex == s.filter.GroupIndex {
		return
	}

	// If the category bits change, the proxy needs to be destroyed because it
	// affects the tree sorting.
	destroyProxy := filter.CategoryBits != s.filter.CategoryBits

	s.filter = filter

	// need to wake bodies because a filter change may destroy contacts
	wakeBodies := true
	w.resetProxy(s, wakeBodies, destroyProxy)

	// note: this does not immediately update sensor overlaps. Instead sensor
	// overlaps are updated the next time step
}

// EnableShapeSensorEvents enables sensor events for this shape
// (upstream b2Shape_EnableSensorEvents).
func (w *World) EnableShapeSensorEvents(shapeID ShapeID, flag bool) {
	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	s.enableSensorEvents = flag
}

// AreShapeSensorEventsEnabled reports whether sensor events are enabled
// (upstream b2Shape_AreSensorEventsEnabled).
func (w *World) AreShapeSensorEventsEnabled(shapeID ShapeID) bool {
	s := w.getShape(shapeID)
	return s.enableSensorEvents
}

// EnableShapeContactEvents enables contact events for this shape
// (upstream b2Shape_EnableContactEvents).
//
// Warning: changing this at run-time may lead to lost begin/end events.
func (w *World) EnableShapeContactEvents(shapeID ShapeID, flag bool) {
	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	s.enableContactEvents = flag
}

// AreShapeContactEventsEnabled reports whether contact events are enabled
// (upstream b2Shape_AreContactEventsEnabled).
func (w *World) AreShapeContactEventsEnabled(shapeID ShapeID) bool {
	s := w.getShape(shapeID)
	return s.enableContactEvents
}

// EnableShapePreSolveEvents enables pre-solve contact events for this shape.
// Only applies to dynamic bodies. These are expensive
// (upstream b2Shape_EnablePreSolveEvents).
func (w *World) EnableShapePreSolveEvents(shapeID ShapeID, flag bool) {
	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	s.enablePreSolveEvents = flag
}

// AreShapePreSolveEventsEnabled reports whether pre-solve events are enabled
// (upstream b2Shape_ArePreSolveEventsEnabled).
func (w *World) AreShapePreSolveEventsEnabled(shapeID ShapeID) bool {
	s := w.getShape(shapeID)
	return s.enablePreSolveEvents
}

// EnableShapeHitEvents enables contact hit events for this shape
// (upstream b2Shape_EnableHitEvents).
func (w *World) EnableShapeHitEvents(shapeID ShapeID, flag bool) {
	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	s.enableHitEvents = flag
}

// AreShapeHitEventsEnabled reports whether hit events are enabled
// (upstream b2Shape_AreHitEventsEnabled).
func (w *World) AreShapeHitEventsEnabled(shapeID ShapeID) bool {
	s := w.getShape(shapeID)
	return s.enableHitEvents
}

// ShapeType returns the type of a shape (upstream b2Shape_GetType).
func (w *World) ShapeType(shapeID ShapeID) ShapeType {
	s := w.getShape(shapeID)
	return s.shapeType
}

// ShapeCircle returns a copy of the shape's circle. Asserts the type is
// correct (upstream b2Shape_GetCircle).
func (w *World) ShapeCircle(shapeID ShapeID) Circle {
	s := w.getShape(shapeID)
	assert(s.shapeType == CircleShape)
	return s.circle
}

// ShapeSegment returns a copy of the shape's line segment. Asserts the type
// is correct (upstream b2Shape_GetSegment).
func (w *World) ShapeSegment(shapeID ShapeID) Segment {
	s := w.getShape(shapeID)
	assert(s.shapeType == SegmentShape)
	return s.segment
}

// ShapeChainSegment returns a copy of the shape's chain segment. These come
// from chain shapes. Asserts the type is correct
// (upstream b2Shape_GetChainSegment).
func (w *World) ShapeChainSegment(shapeID ShapeID) ChainSegment {
	s := w.getShape(shapeID)
	assert(s.shapeType == ChainSegmentShape)
	return s.chainSegment
}

// ShapeCapsule returns a copy of the shape's capsule. Asserts the type is
// correct (upstream b2Shape_GetCapsule).
func (w *World) ShapeCapsule(shapeID ShapeID) Capsule {
	s := w.getShape(shapeID)
	assert(s.shapeType == CapsuleShape)
	return s.capsule
}

// ShapePolygon returns a copy of the shape's convex polygon. Asserts the type
// is correct (upstream b2Shape_GetPolygon).
func (w *World) ShapePolygon(shapeID ShapeID) Polygon {
	s := w.getShape(shapeID)
	assert(s.shapeType == PolygonShape)
	return s.polygon
}

// SetShapeCircle allows you to change a shape to be a circle or update the
// current circle. This does not modify the mass properties
// (upstream b2Shape_SetCircle).
func (w *World) SetShapeCircle(shapeID ShapeID, circle *Circle) {
	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	s.circle = *circle
	s.shapeType = CircleShape
	s.aabbMargin = computeShapeMargin(s)

	// need to wake bodies so they can react to the shape change
	wakeBodies := true
	destroyProxy := true
	w.resetProxy(s, wakeBodies, destroyProxy)
}

// SetShapeCapsule allows you to change a shape to be a capsule or update the
// current capsule. This does not modify the mass properties
// (upstream b2Shape_SetCapsule).
func (w *World) SetShapeCapsule(shapeID ShapeID, capsule *Capsule) {
	if w.locked {
		assert(false)
		return
	}

	lengthSqr := DistanceSquared(capsule.Center1, capsule.Center2)
	if lengthSqr <= LinearSlop*LinearSlop {
		return
	}

	s := w.getShape(shapeID)
	s.capsule = *capsule
	s.shapeType = CapsuleShape
	s.aabbMargin = computeShapeMargin(s)

	// need to wake bodies so they can react to the shape change
	wakeBodies := true
	destroyProxy := true
	w.resetProxy(s, wakeBodies, destroyProxy)
}

// SetShapeSegment allows you to change a shape to be a segment or update the
// current segment (upstream b2Shape_SetSegment).
func (w *World) SetShapeSegment(shapeID ShapeID, segment *Segment) {
	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	s.segment = *segment
	s.shapeType = SegmentShape
	s.aabbMargin = computeShapeMargin(s)

	// need to wake bodies so they can react to the shape change
	wakeBodies := true
	destroyProxy := true
	w.resetProxy(s, wakeBodies, destroyProxy)
}

// SetShapePolygon allows you to change a shape to be a polygon or update the
// current polygon. This does not modify the mass properties
// (upstream b2Shape_SetPolygon).
func (w *World) SetShapePolygon(shapeID ShapeID, polygon *Polygon) {
	// Public API precondition: same reasoning as CreatePolygonShape, this is
	// the other way a caller-built Polygon enters the world.
	requireValidPolygonShapeCount(polygon)

	if w.locked {
		assert(false)
		return
	}

	s := w.getShape(shapeID)
	s.polygon = *polygon
	s.shapeType = PolygonShape
	s.aabbMargin = computeShapeMargin(s)

	// need to wake bodies so they can react to the shape change
	wakeBodies := true
	destroyProxy := true
	w.resetProxy(s, wakeBodies, destroyProxy)
}

// ShapeParentChain returns the parent chain id if the shape type is a chain
// segment, otherwise returns the zero ChainID
// (upstream b2Shape_GetParentChain).
func (w *World) ShapeParentChain(shapeID ShapeID) ChainID {
	s := w.getShape(shapeID)
	if s.shapeType == ChainSegmentShape {
		chainID := s.chainSegment.ChainID
		if chainID != NullIndex {
			chain := &w.chainShapes[chainID]
			return ChainID{index1: int32(chainID + 1), world0: shapeID.world0, generation: chain.generation}
		}
	}

	return ChainID{}
}

// ChainSurfaceMaterialCount returns the number of materials on a chain shape
// (upstream b2Chain_GetSurfaceMaterialCount).
func (w *World) ChainSurfaceMaterialCount(chainID ChainID) int {
	chain := w.getChainShape(chainID)
	return len(chain.materials)
}

// SetChainSurfaceMaterial sets the material on a chain shape at the given
// index (upstream b2Chain_SetSurfaceMaterial).
func (w *World) SetChainSurfaceMaterial(chainID ChainID, material SurfaceMaterial, materialIndex int) {
	if w.locked {
		assert(false)
		return
	}

	chain := w.getChainShape(chainID)
	assert(0 <= materialIndex && materialIndex < len(chain.materials))
	chain.materials[materialIndex] = material

	assert(len(chain.materials) == 1 || len(chain.materials) == len(chain.shapeIndices))
	count := len(chain.shapeIndices)

	if len(chain.materials) == 1 {
		for i := range count {
			shapeID := chain.shapeIndices[i]
			s := &w.shapes[shapeID]
			s.material = material
		}
	} else {
		shapeID := chain.shapeIndices[materialIndex]
		s := &w.shapes[shapeID]
		s.material = material
	}
}

// ChainSurfaceMaterial returns the material on a chain shape for the given
// segment index (upstream b2Chain_GetSurfaceMaterial).
func (w *World) ChainSurfaceMaterial(chainID ChainID, segmentIndex int) SurfaceMaterial {
	chain := w.getChainShape(chainID)
	assert(0 <= segmentIndex && segmentIndex < len(chain.materials))
	return chain.materials[segmentIndex]
}

// ShapeContactCapacity returns the maximum capacity required for retrieving
// all the touching contacts on a shape
// (upstream b2Shape_GetContactCapacity).
func (w *World) ShapeContactCapacity(shapeID ShapeID) int {
	w.panicIfPoisoned()
	if w.locked {
		assert(false)
		return 0
	}

	s := w.getShape(shapeID)
	if s.sensorIndex != NullIndex {
		return 0
	}

	b := &w.bodies[s.bodyID]

	// Conservative and fast
	return b.contactCount
}

// ShapeContactData fills contactData with the touching contact data
// involving a shape, up to len(contactData) elements, and returns the number
// of elements stored (upstream b2Shape_GetContactData).
func (w *World) ShapeContactData(shapeID ShapeID, contactData []ContactData) int {
	w.panicIfPoisoned()
	if w.locked {
		assert(false)
		return 0
	}

	s := w.getShape(shapeID)
	if s.sensorIndex != NullIndex {
		return 0
	}

	b := &w.bodies[s.bodyID]
	contactKey := b.headContactKey
	index := 0
	for contactKey != NullIndex && index < len(contactData) {
		contactID := contactKey >> 1
		edgeIndex := contactKey & 1

		c := &w.contacts[contactID]

		// Does contact involve this shape and is it touching?
		if (c.shapeIDA == int(shapeID.index1)-1 || c.shapeIDB == int(shapeID.index1)-1) &&
			c.flags&contactTouchingFlag != 0 {
			shapeA := &w.shapes[c.shapeIDA]
			shapeB := &w.shapes[c.shapeIDB]

			contactData[index].ContactID = ContactID{
				index1:     int32(c.contactID + 1),
				world0:     shapeID.world0,
				padding:    0,
				generation: c.generation,
			}
			contactData[index].ShapeIDA = ShapeID{
				index1:     int32(shapeA.id + 1),
				world0:     shapeID.world0,
				generation: shapeA.generation,
			}
			contactData[index].ShapeIDB = ShapeID{
				index1:     int32(shapeB.id + 1),
				world0:     shapeID.world0,
				generation: shapeB.generation,
			}

			contactSim := w.getContactSim(c)
			contactData[index].Manifold = contactSim.manifold
			index++
		}

		contactKey = c.edges[edgeIndex].nextKey
	}

	assert(index <= len(contactData))

	return index
}

// ShapeSensorCapacity returns the maximum capacity required for retrieving
// all the overlapped shapes on a sensor shape. This returns 0 if the provided
// shape is not a sensor (upstream b2Shape_GetSensorCapacity).
func (w *World) ShapeSensorCapacity(shapeID ShapeID) int {
	w.panicIfPoisoned()
	if w.locked {
		assert(false)
		return 0
	}

	s := w.getShape(shapeID)
	if s.sensorIndex == NullIndex {
		return 0
	}

	sen := &w.sensors[s.sensorIndex]
	return len(sen.overlaps2)
}

// ShapeSensorData fills visitorIDs with the overlapped shapes for a sensor
// shape, up to len(visitorIDs), and returns the number of ids stored.
// Overlaps may contain destroyed shapes so use IsShapeValid to confirm each
// overlap (upstream b2Shape_GetSensorData).
func (w *World) ShapeSensorData(shapeID ShapeID, visitorIDs []ShapeID) int {
	w.panicIfPoisoned()
	if w.locked {
		assert(false)
		return 0
	}

	s := w.getShape(shapeID)
	if s.sensorIndex == NullIndex {
		return 0
	}

	sen := &w.sensors[s.sensorIndex]

	count := minInt(len(sen.overlaps2), len(visitorIDs))
	refs := sen.overlaps2
	for i := range count {
		visitorIDs[i] = ShapeID{
			index1:     int32(refs[i].shapeID + 1),
			world0:     shapeID.world0,
			generation: refs[i].generation,
		}
	}

	return count
}

// ShapeAABB returns the current world AABB of a shape
// (upstream b2Shape_GetAABB).
func (w *World) ShapeAABB(shapeID ShapeID) AABB {
	s := w.getShape(shapeID)
	return s.aabb
}

// ShapeComputeMassData computes the mass data for a shape
// (upstream b2Shape_ComputeMassData).
func (w *World) ShapeComputeMassData(shapeID ShapeID) MassData {
	s := w.getShape(shapeID)
	return computeShapeMass(s)
}

// ShapeClosestPoint returns the closest point on a shape to a target point.
// Target and result are in world space (upstream b2Shape_GetClosestPoint).
func (w *World) ShapeClosestPoint(shapeID ShapeID, target Vec2) Vec2 {
	s := w.getShape(shapeID)
	b := &w.bodies[s.bodyID]
	transform := w.getBodyTransformQuick(b)

	var input DistanceInput
	input.ProxyA = makeShapeDistanceProxy(s)
	input.ProxyB = MakeProxy([]Vec2{target}, 1, 0.0)
	input.TransformA = transform
	input.TransformB = TransformIdentity
	input.UseRadii = true

	cache := SimplexCache{}
	output := ShapeDistance(&input, &cache, nil)

	return output.PointA
}

// ApplyShapeWind applies a wind force to a shape considering its exposed
// perimeter and velocity (upstream b2Shape_ApplyWind).
//
// https://en.wikipedia.org/wiki/Density_of_air
// https://www.engineeringtoolbox.com/wind-load-d_1775.html
// force = 0.5 * air_density * velocity^2 * area
// https://en.wikipedia.org/wiki/Lift_(force)
func (w *World) ApplyShapeWind(shapeID ShapeID, wind Vec2, drag, lift float64, wake bool) {
	s := w.getShape(shapeID)

	shapeType := s.shapeType
	if shapeType != CircleShape && shapeType != CapsuleShape && shapeType != PolygonShape {
		return
	}

	b := &w.bodies[s.bodyID]

	if b.bodyType != DynamicBody {
		return
	}

	if b.setIndex >= firstSleepingSet && !wake {
		return
	}

	sim := w.getBodySim(b)

	if b.setIndex != awakeSet {
		// Must wake for state to exist
		w.wakeBody(b)
	}

	assert(b.setIndex == awakeSet)

	state := w.getBodyState(b)
	transform := sim.transform

	lengthUnits := GetLengthUnitsPerMeter()
	volumeUnits := lengthUnits * lengthUnits * lengthUnits

	// In 2D unit depth is assumed
	airDensity := 1.2250 / volumeUnits

	force := Vec2Zero
	torque := 0.0

	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CircleShape:
		radius := s.circle.Radius
		centroid := s.localCentroid
		lever := RotateVector(transform.Q, Sub(centroid, sim.localCenter))
		shapeVelocity := Add(state.linearVelocity, CrossSV(state.angularVelocity, lever))
		relativeVelocity := MulSub(wind, drag, shapeVelocity)
		direction, speed := GetLengthAndNormalize(relativeVelocity)
		projectedArea := 2.0 * radius
		force = MulSV(0.5*airDensity*projectedArea*speed*speed, direction)
		torque = Cross(lever, force)

	case CapsuleShape:
		centroid := s.localCentroid
		lever := RotateVector(transform.Q, Sub(centroid, sim.localCenter))
		shapeVelocity := Add(state.linearVelocity, CrossSV(state.angularVelocity, lever))
		relativeVelocity := MulSub(wind, drag, shapeVelocity)
		direction, speed := GetLengthAndNormalize(relativeVelocity)

		d := Sub(s.capsule.Center2, s.capsule.Center1)
		d = RotateVector(transform.Q, d)

		radius := s.capsule.Radius
		projectedArea := mulAdd(2.0, radius, absFloat(Cross(d, direction)))

		// Normal that opposes the wind
		normal := LeftPerp(Normalize(d))
		if Dot(normal, direction) > 0.0 {
			normal = Neg(normal)
		}

		// portion of wind that is perpendicular to surface
		liftDirection := CrossSV(Cross(normal, direction), direction)

		forceMagnitude := 0.5 * airDensity * projectedArea * speed * speed
		force = MulSV(forceMagnitude, MulAdd(direction, lift, liftDirection))

		edgeLever := MulAdd(lever, radius, normal)
		torque = Cross(edgeLever, force)

	case PolygonShape:
		centroid := s.localCentroid
		lever := RotateVector(transform.Q, Sub(centroid, sim.localCenter))
		shapeVelocity := Add(state.linearVelocity, CrossSV(state.angularVelocity, lever))
		relativeVelocity := MulSub(wind, drag, shapeVelocity)
		direction, speed := GetLengthAndNormalize(relativeVelocity)

		// polygon radius is ignored for simplicity
		count := s.polygon.Count
		vertices := s.polygon.Vertices

		v1 := vertices[count-1]
		for i := range count {
			v2 := vertices[i]
			d := Sub(v2, v1)
			edgeCenter := Lerp(v1, v2, 0.5)
			v1 = v2

			d = RotateVector(transform.Q, d)

			projectedArea := Cross(d, direction)
			if projectedArea <= 0.0 {
				// back facing
				continue
			}

			normal := RightPerp(Normalize(d))

			// portion of wind that is perpendicular to surface
			liftDirection := CrossSV(Cross(normal, direction), direction)

			forceMagnitude := 0.5 * airDensity * projectedArea * speed * speed
			f := MulSV(forceMagnitude, MulAdd(direction, lift, liftDirection))

			edgeLever := RotateVector(transform.Q, Sub(edgeCenter, sim.localCenter))

			force = Add(force, f)
			torque += Cross(edgeLever, f)
		}

	default:
	}

	sim.force = Add(sim.force, force)
	sim.torque += torque
}

// getShapeRadius returns the round radius of a shape (upstream
// b2GetShapeRadius).
func getShapeRadius(s *shape) float64 {
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		return s.capsule.Radius
	case CircleShape:
		return s.circle.Radius
	case PolygonShape:
		return s.polygon.Radius
	default:
		return 0.0
	}
}

// shouldShapesCollide reports whether two shape filters allow collision
// (upstream b2ShouldShapesCollide).
func shouldShapesCollide(filterA, filterB Filter) bool {
	if filterA.GroupIndex == filterB.GroupIndex && filterA.GroupIndex != 0 {
		return filterA.GroupIndex > 0
	}

	return filterA.MaskBits&filterB.CategoryBits != 0 && filterA.CategoryBits&filterB.MaskBits != 0
}

// shouldQueryCollide reports whether a query filter allows collision with a
// shape filter (upstream b2ShouldQueryCollide).
func shouldQueryCollide(shapeFilter Filter, queryFilter QueryFilter) bool {
	return shapeFilter.CategoryBits&queryFilter.MaskBits != 0 && shapeFilter.MaskBits&queryFilter.CategoryBits != 0
}
