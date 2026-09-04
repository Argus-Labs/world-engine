// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file
// src/physics_world.c (debug draw section: b2DrawShape, DrawContext,
// DrawQueryCallback, b2World_Draw) plus b2_graphColors from
// src/constraint_graph.c.
//
// DESIGN DEVIATION (approved, see world.go): upstream resolves the world
// through the global b2_worlds array via b2GetWorldFromId. This port has no
// world registry, so b2World_Draw becomes a method on *World and the receiver
// replaces the lookup. The `void* context` of the tree callback becomes an
// `any` value carrying *drawContext.
//
// Other deviations from upstream:
//   - The upstream snprintf calls that format body mass, contact separation,
//     contact forces and contact feature ids become strconv.FormatFloat /
//     strconv.Itoa with the same precision, so no fmt dependency enters the
//     simulation package.
//   - The b2Polygon vertex pointer+count pair becomes a slice
//     (poly.Vertices[:poly.Count]) to match DebugDraw.DrawSolidPolygonFcn.
//   - Upstream's island loop uses `continue` for an island with a null set
//     index, which skips the "clear the lowest set bit" statement of the
//     enclosing while loop. This port skips only the island drawing so the
//     body word always makes progress.
//   - Joint drawing is wired in drawBody, matching the upstream b2World_Draw
//     joint loop guarded by world.debugJointSet.

package box2d

import (
	"math"
	"strconv"
)

// graphColorPalette is the debug draw palette indexed by constraint graph
// color (upstream b2_graphColors in src/constraint_graph.c). The last entry is
// the overflow color.
var graphColorPalette = [GraphColorCount]HexColor{
	ColorRed, ColorOrange, ColorYellow, ColorGreen, ColorCyan, ColorBlue,
	ColorViolet, ColorPink, ColorChocolate, ColorGoldenRod, ColorCoral, ColorRosyBrown,
	ColorAqua, ColorPeru, ColorLime, ColorGold, ColorPlum, ColorSnow,
	ColorTeal, ColorKhaki, ColorSalmon, ColorPeachPuff, ColorHoneyDew, ColorBlack,
}

// drawShape renders a single shape with the supplied transform and color
// (upstream static b2DrawShape).
func drawShape(draw *DebugDraw, s *shape, xf Transform, color HexColor) {
	//nolint:exhaustive // ShapeTypeCount is a sentinel, not a shape type; the default case mirrors upstream.
	switch s.shapeType {
	case CapsuleShape:
		capsule := &s.capsule
		p1 := TransformPoint(xf, capsule.Center1)
		p2 := TransformPoint(xf, capsule.Center2)
		draw.DrawSolidCapsuleFcn(p1, p2, capsule.Radius, color, draw.Context)

	case CircleShape:
		circle := &s.circle
		xf.P = TransformPoint(xf, circle.Center)
		draw.DrawSolidCircleFcn(xf, circle.Radius, color, draw.Context)

	case PolygonShape:
		poly := &s.polygon
		draw.DrawSolidPolygonFcn(xf, poly.Vertices[:poly.Count], poly.Radius, color, draw.Context)

	case SegmentShape:
		segment := &s.segment
		p1 := TransformPoint(xf, segment.Point1)
		p2 := TransformPoint(xf, segment.Point2)
		draw.DrawLineFcn(p1, p2, color, draw.Context)

	case ChainSegmentShape:
		segment := &s.chainSegment.Segment
		p1 := TransformPoint(xf, segment.Point1)
		p2 := TransformPoint(xf, segment.Point2)
		draw.DrawLineFcn(p1, p2, color, draw.Context)
		draw.DrawPointFcn(p2, 4.0, color, draw.Context)
		draw.DrawLineFcn(p1, Lerp(p1, p2, 0.1), ColorPaleGreen, draw.Context)

	default:
	}
}

// drawContext is the callback context for World.Draw (upstream struct
// DrawContext).
type drawContext struct {
	world *World
	draw  *DebugDraw
}

// drawQueryCallback marks the owning body for the second pass and draws the
// shape and its fat AABB (upstream static DrawQueryCallback).
func drawQueryCallback(proxyID int, userData uint64, context any) bool {
	_ = proxyID

	shapeID := int(userData)

	ctx, ok := context.(*drawContext)
	assert(ok)
	if !ok {
		return false
	}
	world := ctx.world
	draw := ctx.draw

	s := &world.shapes[shapeID]
	assert(s.id == shapeID)

	setBit(&world.debugBodySet, uint32(s.bodyID))

	if draw.DrawShapes {
		b := &world.bodies[s.bodyID]
		sim := world.getBodySim(b)

		color := shapeDrawColor(b, sim, s)

		drawShape(draw, s, sim.transform, color)
	}

	if draw.DrawBounds {
		aabb := s.fatAABB
		draw.DrawPolygonFcn(aabbCorners(aabb), ColorGold, draw.Context)
	}

	return true
}

// shapeDrawColor picks the debug color for a shape (upstream: the color
// if/else ladder inside DrawQueryCallback).
func shapeDrawColor(b *body, sim *bodySim, s *shape) HexColor {
	switch {
	case s.material.CustomColor != 0:
		return HexColor(s.material.CustomColor)
	case b.bodyType == DynamicBody && b.mass == 0.0:
		// Bad body
		return ColorRed
	case b.setIndex == disabledSet:
		return ColorSlateGray
	case s.sensorIndex != NullIndex:
		return ColorWheat
	case b.flags&hadTimeOfImpact != 0:
		return ColorLime
	case sim.flags&isBullet != 0 && b.setIndex == awakeSet:
		return ColorTurquoise
	case b.flags&isSpeedCapped != 0:
		return ColorYellow
	case sim.flags&isFast != 0:
		return ColorSalmon
	case b.bodyType == StaticBody:
		return ColorPaleGreen
	case b.bodyType == KinematicBody:
		return ColorRoyalBlue
	case b.setIndex == awakeSet:
		return ColorPink
	default:
		return ColorGray
	}
}

// aabbCorners returns the four CCW corners of an AABB. Upstream builds this
// b2Vec2 vs[4] literal inline in DrawQueryCallback and in the island loop.
func aabbCorners(aabb AABB) []Vec2 {
	return []Vec2{
		{X: aabb.LowerBound.X, Y: aabb.LowerBound.Y},
		{X: aabb.UpperBound.X, Y: aabb.LowerBound.Y},
		{X: aabb.UpperBound.X, Y: aabb.UpperBound.Y},
		{X: aabb.LowerBound.X, Y: aabb.UpperBound.Y},
	}
}

// axisScale is the debug draw length of a contact normal (upstream local
// k_axisScale).
const axisScale = 0.3

// Debug draw colors for contact points and contact vectors (upstream locals in
// b2World_Draw).
const (
	speculativeColor = ColorGainsboro
	addColor         = ColorGreen
	persistColor     = ColorBlue
	normalColor      = ColorDimGray
	impulseColor     = ColorMagenta
	frictionColor    = ColorYellow
)

// Draw calls the user-supplied debug draw callbacks for every shape, contact,
// mass center and island that overlaps draw.DrawingBounds
// (upstream b2World_Draw).
//
// Shapes are gathered with a broad-phase query per body type, which sets one
// bit per touched body; the second pass then walks the body bit set so each
// body is visited exactly once regardless of how many shapes it owns.
func (w *World) Draw(draw *DebugDraw) {
	w.panicIfPoisoned()
	assert(!w.locked)
	if w.locked {
		return
	}

	if debugAsserts {
		assert(IsValidAABB(draw.DrawingBounds))
	}

	bodyCapacity := getIDCapacity(&w.bodyIDPool)
	setBitCountAndClear(&w.debugBodySet, uint32(bodyCapacity))

	jointCapacity := getIDCapacity(&w.jointIDPool)
	setBitCountAndClear(&w.debugJointSet, uint32(jointCapacity))

	contactCapacity := getIDCapacity(&w.contactIDPool)
	setBitCountAndClear(&w.debugContactSet, uint32(contactCapacity))

	islandCapacity := getIDCapacity(&w.islandIDPool)
	setBitCountAndClear(&w.debugIslandSet, uint32(islandCapacity))

	ctx := drawContext{world: w, draw: draw}

	for i := range int(BodyTypeCount) {
		w.broadPhase.trees[i].QueryAll(draw.DrawingBounds, drawQueryCallback, &ctx)
	}

	bits := w.debugBodySet.bits
	for k := range w.debugBodySet.blockCount {
		word := bits[k]
		for word != 0 {
			bodyID := int(64*k + ctz64(word))

			w.drawBody(draw, bodyID)

			// Clear the smallest set bit.
			word &= word - 1
		}
	}
}

// drawBody renders the per-body debug information for one body found by the
// broad-phase pass (upstream: the body of the bit-scan loop in b2World_Draw).
func (w *World) drawBody(draw *DebugDraw, bodyID int) {
	b := &w.bodies[bodyID]

	if draw.DrawBodyNames && b.name != "" {
		offset := Vec2{X: 0.1, Y: 0.1}
		sim := w.getBodySim(b)

		transform := Transform{P: sim.center, Q: sim.transform.Q}
		p := TransformPoint(transform, offset)
		draw.DrawStringFcn(p, b.name, ColorBlueViolet, draw.Context)
	}

	if draw.DrawMass && b.bodyType == DynamicBody {
		offset := Vec2{X: 0.1, Y: 0.1}
		sim := w.getBodySim(b)

		transform := Transform{P: sim.center, Q: sim.transform.Q}
		draw.DrawLineFcn(sim.center0, sim.center, ColorWhiteSmoke, draw.Context)
		draw.DrawTransformFcn(transform, draw.Context)

		p := TransformPoint(transform, offset)
		draw.DrawStringFcn(p, "  "+strconv.FormatFloat(b.mass, 'f', 2, 64), ColorWhite, draw.Context)
	}

	if draw.DrawJoints {
		jointKey := b.headJointKey
		for jointKey != NullIndex {
			jointID := jointKey >> 1
			edgeIndex := jointKey & 1
			j := &w.joints[jointID]

			// avoid double draw
			if getBit(&w.debugJointSet, uint32(jointID)) {
				jointKey = j.edges[edgeIndex].nextKey
				continue
			}
			setBit(&w.debugJointSet, uint32(jointID))

			w.drawJoint(draw, j)

			jointKey = j.edges[edgeIndex].nextKey
		}
	}

	if draw.ContactDrawType != DrawContactsNone && b.bodyType == DynamicBody {
		w.drawBodyContacts(draw, b)
	}

	if draw.DrawIslands {
		w.drawBodyIsland(draw, b)
	}
}

// drawBodyContacts renders the contact points and contact vectors of one body
// (upstream: the contactDrawType block in b2World_Draw).
func (w *World) drawBodyContacts(draw *DebugDraw, b *body) {
	contactKey := b.headContactKey
	for contactKey != NullIndex {
		contactID := contactKey >> 1
		edgeIndex := contactKey & 1
		c := &w.contacts[contactID]
		contactKey = c.edges[edgeIndex].nextKey

		// avoid double draw
		if getBit(&w.debugContactSet, uint32(contactID)) {
			continue
		}
		setBit(&w.debugContactSet, uint32(contactID))

		sim := w.getContactSim(c)
		bodyA := &w.bodies[c.edges[0].bodyID]
		simA := w.getBodySim(bodyA)
		bodyB := &w.bodies[c.edges[1].bodyID]
		simB := w.getBodySim(bodyB)
		normal := sim.manifold.Normal

		for j := range sim.manifold.PointCount {
			mp := &sim.manifold.Points[j]

			p := mp.ClipPoint
			switch draw.ContactDrawType {
			case DrawContactsAnchorA:
				p = Add(simA.center, mp.AnchorA)
			case DrawContactsAnchorB:
				p = Add(simB.center, mp.AnchorB)
			case DrawContactsAverage:
				pA := Add(simA.center, mp.AnchorA)
				pB := Add(simB.center, mp.AnchorB)
				p = Lerp(pA, pB, 0.5)
			case DrawContactsNone, DrawContactsClip:
			}

			switch {
			case draw.DrawGraphColors && c.colorIndex != NullIndex:
				// graph color
				pointSize := 5.0
				if c.colorIndex == overflowIndex {
					pointSize = 7.5
				}
				draw.DrawPointFcn(p, pointSize, graphColorPalette[c.colorIndex], draw.Context)
			case mp.Separation > LinearSlop:
				// Speculative
				draw.DrawPointFcn(p, 5.0, speculativeColor, draw.Context)
			case !mp.Persisted:
				// Add
				draw.DrawPointFcn(p, 10.0, addColor, draw.Context)
			default:
				// Persist
				draw.DrawPointFcn(p, 5.0, persistColor, draw.Context)
			}

			w.drawContactVectors(draw, mp, p, normal)
		}
	}
}

// drawContactVectors renders the normal, force, feature id and friction force
// annotations of one manifold point (upstream: the tail of the manifold point
// loop in b2World_Draw).
func (w *World) drawContactVectors(draw *DebugDraw, mp *ManifoldPoint, p Vec2, normal Vec2) {
	switch {
	case draw.DrawContactNormals:
		p1 := p
		p2 := MulAdd(p1, axisScale, normal)
		draw.DrawLineFcn(p1, p2, normalColor, draw.Context)
		draw.DrawStringFcn(p1, " "+strconv.FormatFloat(mp.Separation, 'f', 2, 64), ColorWhite, draw.Context)

	case draw.DrawContactForces:
		// multiply by one-half due to relax iteration
		force := 0.5 * mp.TotalNormalImpulse * w.invDt
		p1 := p
		p2 := MulAdd(p1, draw.ForceScale*force, normal)
		draw.DrawLineFcn(p1, p2, impulseColor, draw.Context)
		draw.DrawStringFcn(p1, strconv.FormatFloat(force, 'f', 1, 64), ColorWhite, draw.Context)
	}

	if draw.DrawContactFeatures {
		draw.DrawStringFcn(p, strconv.Itoa(int(mp.ID)), ColorOrange, draw.Context)
	}

	if draw.DrawFrictionForces {
		force := 0.5 * mp.TangentImpulse * w.invH
		tangent := RightPerp(normal)
		p1 := p
		p2 := MulAdd(p1, draw.ForceScale*force, tangent)
		draw.DrawLineFcn(p1, p2, frictionColor, draw.Context)
		draw.DrawStringFcn(p1, strconv.FormatFloat(force, 'f', 1, 64), ColorWhite, draw.Context)
	}
}

// drawBodyIsland renders the bounding box of the island owning this body
// (upstream: the drawIslands block in b2World_Draw).
func (w *World) drawBodyIsland(draw *DebugDraw, b *body) {
	islandID := b.islandID
	if islandID == NullIndex || getBit(&w.debugIslandSet, uint32(islandID)) {
		return
	}

	isl := &w.islands[islandID]
	if isl.setIndex == NullIndex {
		// Deviation: upstream `continue`s the enclosing bit-scan loop here,
		// which would skip the bit clear. Skipping just the island draw keeps
		// the loop making progress and leaves the island bit unset, matching
		// the upstream intent.
		return
	}

	shapeCount := 0
	aabb := AABB{
		LowerBound: Vec2{X: math.MaxFloat32, Y: math.MaxFloat32},
		UpperBound: Vec2{X: -math.MaxFloat32, Y: -math.MaxFloat32},
	}

	for _, islandBodyID := range isl.bodies {
		islandBody := &w.bodies[islandBodyID]
		shapeID := islandBody.headShapeID
		for shapeID != NullIndex {
			s := &w.shapes[shapeID]
			aabb = AABBUnion(aabb, s.fatAABB)
			shapeCount++
			shapeID = s.nextShapeID
		}
	}

	if shapeCount > 0 {
		draw.DrawPolygonFcn(aabbCorners(aabb), ColorOrangeRed, draw.Context)
	}

	setBit(&w.debugIslandSet, uint32(islandID))
}
