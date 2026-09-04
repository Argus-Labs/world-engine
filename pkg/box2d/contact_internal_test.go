// Tests for the float64 port of Box2D v3.2.0 src/contact.c, src/island.c
// (contact linking), src/constraint_graph.c (contact coloring) and the
// src/broad_phase.c pair machinery (updateBroadPhasePairs). Internal package
// tests because they inspect contact, island, solver set and constraint
// graph internals directly.
//
// The narrow-phase state transitions (started/stopped touching) belong to
// the E7 solver (physics_world.c b2Collide). The helpers below mimic those
// transitions so the E6 machinery can be exercised end-to-end.

package box2d

import (
	"math"
	"testing"

	tassert "github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// promoteTouchingContacts mimics the b2_simStartedTouching transition of
// upstream b2Collide (physics_world.c): update every awake non-touching
// contact and, when touching, link it into an island and move it into the
// constraint graph. Contacts are processed in awake-set array order.
func promoteTouchingContacts(t *testing.T, w *World) {
	t.Helper()

	awake := &w.solverSets[awakeSet]
	i := 0
	for i < len(awake.contactSims) {
		cSim := &awake.contactSims[i]
		c := &w.contacts[cSim.contactID]

		shapeA := &w.shapes[cSim.shapeIDA]
		shapeB := &w.shapes[cSim.shapeIDB]
		transformA := w.getBodyTransform(shapeA.bodyID)
		transformB := w.getBodyTransform(shapeB.bodyID)

		touching := w.updateContact(cSim, shapeA, transformA, Vec2Zero, shapeB, transformB, Vec2Zero)
		if !touching {
			i++
			continue
		}

		require.Equal(t, awakeSet, c.setIndex)
		require.Equal(t, NullIndex, c.islandID)

		// Link first because this wakes colliding bodies (upstream comment).
		c.flags |= contactTouchingFlag
		w.linkContact(c)

		require.Equal(t, NullIndex, c.colorIndex)
		require.Equal(t, i, c.localIndex)

		// Refresh the sim pointer (awake set may have grown) and add to the
		// constraint graph, then remove the non-touching sim
		// (upstream b2RemoveNonTouchingContact).
		cSim = &awake.contactSims[i]
		w.addContactToGraph(cSim, c)

		movedIndex := removeSwap(&awake.contactSims, i)
		if movedIndex != NullIndex {
			movedContact := &w.contacts[awake.contactSims[i].contactID]
			movedContact.localIndex = i
		}
	}
}

// demoteContact mimics the b2_simStoppedTouching transition of upstream
// b2Collide: unlink the contact from its island and move it from the
// constraint graph back to the awake set as non-touching.
func demoteContact(w *World, c *contact) {
	colorIndex := c.colorIndex
	localIndex := c.localIndex

	color := &w.constraintGraph.colors[colorIndex]
	cSim := color.contactSims[localIndex]

	c.flags &^= contactTouchingFlag
	w.unlinkContact(c)

	// upstream b2AddNonTouchingContact
	awake := &w.solverSets[awakeSet]
	c.colorIndex = NullIndex
	c.localIndex = len(awake.contactSims)
	awake.contactSims = append(awake.contactSims, cSim)

	w.removeContactFromGraph(c.edges[0].bodyID, c.edges[1].bodyID, colorIndex, localIndex)
}

// addDynamicCircle creates a dynamic body with one circle shape of radius 0.5.
func addDynamicCircle(w *World, x, y float64, shapeDef *ShapeDef) (BodyID, ShapeID) {
	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyDef.Position = Vec2{X: x, Y: y}
	bodyID := w.CreateBody(&bodyDef)
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	shapeID := w.CreateCircleShape(bodyID, shapeDef, &circle)
	return bodyID, shapeID
}

func TestContactRegistersCoverAllShapeTypePairs(t *testing.T) {
	// Pairs without a manifold function, mirroring upstream s_registers:
	// segment-family shapes never collide with each other.
	nilPairs := map[[2]ShapeType]bool{
		{SegmentShape, SegmentShape}:           true,
		{SegmentShape, ChainSegmentShape}:      true,
		{ChainSegmentShape, SegmentShape}:      true,
		{ChainSegmentShape, ChainSegmentShape}: true,
	}

	for typeA := ShapeType(0); typeA < ShapeTypeCount; typeA++ {
		for typeB := ShapeType(0); typeB < ShapeTypeCount; typeB++ {
			entry := contactRegisters[typeA][typeB]
			mirror := contactRegisters[typeB][typeA]

			if nilPairs[[2]ShapeType{typeA, typeB}] {
				tassert.Nil(t, entry.fcn, "types %d/%d", typeA, typeB)
				tassert.False(t, entry.primary, "types %d/%d", typeA, typeB)
				tassert.False(t, canCollide(typeA, typeB), "types %d/%d", typeA, typeB)
				continue
			}

			require.NotNil(t, entry.fcn, "types %d/%d", typeA, typeB)
			tassert.True(t, canCollide(typeA, typeB), "types %d/%d", typeA, typeB)

			// Exactly one direction of an asymmetric pair is primary; the
			// diagonal is primary.
			if typeA == typeB {
				tassert.True(t, entry.primary, "types %d/%d", typeA, typeB)
			} else {
				tassert.NotEqual(t, entry.primary, mirror.primary, "types %d/%d", typeA, typeB)
			}
		}
	}

	// Spot-check upstream b2AddType order: first argument is primary.
	tassert.True(t, contactRegisters[CapsuleShape][CircleShape].primary)
	tassert.False(t, contactRegisters[CircleShape][CapsuleShape].primary)
	tassert.True(t, contactRegisters[PolygonShape][CapsuleShape].primary)
	tassert.True(t, contactRegisters[ChainSegmentShape][PolygonShape].primary)
	tassert.False(t, contactRegisters[PolygonShape][ChainSegmentShape].primary)
}

func TestUpdateBroadPhasePairsCreatesContact(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()
	bodyIDA, shapeIDA := addDynamicCircle(w, 0.0, 0.0, &shapeDef)
	bodyIDB, shapeIDB := addDynamicCircle(w, 0.5, 0.0, &shapeDef)

	require.Len(t, w.broadPhase.moveArray, 2)

	w.updateBroadPhasePairs()

	// Exactly one contact between the right shape pair.
	require.Equal(t, 1, w.Counters().ContactCount)
	c := &w.contacts[0]
	tassert.Equal(t, 0, c.contactID)
	tassert.Equal(t, int(shapeIDA.index1)-1, c.shapeIDA)
	tassert.Equal(t, int(shapeIDB.index1)-1, c.shapeIDB)

	// Created as non-touching in the awake set, not in an island or the graph.
	tassert.Equal(t, awakeSet, c.setIndex)
	tassert.Equal(t, NullIndex, c.colorIndex)
	tassert.Equal(t, NullIndex, c.islandID)
	tassert.Equal(t, NullIndex, c.islandIndex)
	tassert.Equal(t, uint32(0), c.flags)

	awake := &w.solverSets[awakeSet]
	require.Len(t, awake.contactSims, 1)
	tassert.Equal(t, 0, c.localIndex)
	sim := &awake.contactSims[c.localIndex]
	tassert.Equal(t, c.contactID, sim.contactID)
	tassert.Equal(t, c.shapeIDA, sim.shapeIDA)
	tassert.Equal(t, c.shapeIDB, sim.shapeIDB)
	tassert.Equal(t, 0, sim.manifold.PointCount)

	// Default mixing: friction sqrt, restitution max. Compute the expectation
	// through float64 variables so it rounds exactly like the world callback
	// (untyped constant folding is exact and would differ).
	frictionA, frictionB := shapeDef.Material.Friction, shapeDef.Material.Friction
	tassert.InDelta(t, math.Sqrt(frictionA*frictionB), sim.friction, 0.0)
	tassert.InDelta(t, 0.0, sim.restitution, 0.0)

	// Edges are linked into both body contact lists.
	bodyA := w.getBodyFullID(bodyIDA)
	bodyB := w.getBodyFullID(bodyIDB)
	tassert.Equal(t, 1, bodyA.contactCount)
	tassert.Equal(t, 1, bodyB.contactCount)
	tassert.Equal(t, 0, bodyA.headContactKey) // (contactID << 1) | 0
	tassert.Equal(t, 1, bodyB.headContactKey) // (contactID << 1) | 1
	tassert.Equal(t, bodyA.id, c.edges[0].bodyID)
	tassert.Equal(t, bodyB.id, c.edges[1].bodyID)
	tassert.Equal(t, NullIndex, c.edges[0].prevKey)
	tassert.Equal(t, NullIndex, c.edges[0].nextKey)
	tassert.Equal(t, NullIndex, c.edges[1].prevKey)
	tassert.Equal(t, NullIndex, c.edges[1].nextKey)

	// Pair set tracks the pair; move buffer was consumed.
	pairKey := shapePairKey(uint32(c.shapeIDA), uint32(c.shapeIDB))
	tassert.True(t, containsKey(&w.broadPhase.pairSet, pairKey))
	tassert.Empty(t, w.broadPhase.moveArray)
	tassert.Equal(t, 0, getSetCount(&w.broadPhase.moveSet))

	// Re-running with an empty move buffer changes nothing.
	w.updateBroadPhasePairs()
	tassert.Equal(t, 1, w.Counters().ContactCount)

	// Re-buffering the same proxies does not duplicate the existing pair.
	s := w.getShape(shapeIDA)
	w.broadPhase.bufferMove(s.proxyKey)
	s = w.getShape(shapeIDB)
	w.broadPhase.bufferMove(s.proxyKey)
	w.updateBroadPhasePairs()
	tassert.Equal(t, 1, w.Counters().ContactCount)
	require.Len(t, awake.contactSims, 1)
}

func TestCreateContactFlipsNonPrimaryPair(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()

	// Circle body created first, capsule body second. Whatever order the
	// pair query reports, the contact must store the primary order
	// (capsule first).
	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	bodyDef.Position = Vec2Zero
	circleBody := w.CreateBody(&bodyDef)
	circle := Circle{Center: Vec2Zero, Radius: 0.5}
	circleShape := w.CreateCircleShape(circleBody, &shapeDef, &circle)

	bodyDef.Position = Vec2{X: 0.4, Y: 0.0}
	capsuleBody := w.CreateBody(&bodyDef)
	capsule := Capsule{Center1: Vec2{X: -0.25, Y: 0.0}, Center2: Vec2{X: 0.25, Y: 0.0}, Radius: 0.25}
	capsuleShape := w.CreateCapsuleShape(capsuleBody, &shapeDef, &capsule)

	w.updateBroadPhasePairs()

	require.Equal(t, 1, w.Counters().ContactCount)
	c := &w.contacts[0]
	tassert.Equal(t, int(capsuleShape.index1)-1, c.shapeIDA)
	tassert.Equal(t, int(circleShape.index1)-1, c.shapeIDB)
	tassert.Equal(t, w.getBodyFullID(capsuleBody).id, c.edges[0].bodyID)
	tassert.Equal(t, w.getBodyFullID(circleBody).id, c.edges[1].bodyID)
}

func TestUpdateBroadPhasePairsFiltering(t *testing.T) {
	t.Run("DisjointMaskBits", func(t *testing.T) {
		def := DefaultWorldDef()
		w := NewWorld(&def)
		defer w.Destroy()

		shapeDefA := DefaultShapeDef()
		shapeDefA.Filter.CategoryBits = 0x2
		shapeDefA.Filter.MaskBits = 0x4
		shapeDefB := DefaultShapeDef()
		shapeDefB.Filter.CategoryBits = 0x8
		shapeDefB.Filter.MaskBits = 0x1

		_, _ = addDynamicCircle(w, 0.0, 0.0, &shapeDefA)
		_, _ = addDynamicCircle(w, 0.5, 0.0, &shapeDefB)

		w.updateBroadPhasePairs()

		tassert.Equal(t, 0, w.Counters().ContactCount)
		tassert.Equal(t, 0, getSetCount(&w.broadPhase.pairSet))
	})

	t.Run("SameNegativeGroup", func(t *testing.T) {
		def := DefaultWorldDef()
		w := NewWorld(&def)
		defer w.Destroy()

		shapeDef := DefaultShapeDef()
		shapeDef.Filter.GroupIndex = -7

		_, _ = addDynamicCircle(w, 0.0, 0.0, &shapeDef)
		_, _ = addDynamicCircle(w, 0.5, 0.0, &shapeDef)

		w.updateBroadPhasePairs()

		tassert.Equal(t, 0, w.Counters().ContactCount)
	})

	t.Run("SensorShapesSkipped", func(t *testing.T) {
		def := DefaultWorldDef()
		w := NewWorld(&def)
		defer w.Destroy()

		solidDef := DefaultShapeDef()
		sensorDef := DefaultShapeDef()
		sensorDef.IsSensor = true
		sensorDef.EnableSensorEvents = true

		_, _ = addDynamicCircle(w, 0.0, 0.0, &solidDef)
		_, sensorShape := addDynamicCircle(w, 0.5, 0.0, &sensorDef)

		require.NotEqual(t, NullIndex, w.getShape(sensorShape).sensorIndex)

		// v3.2 sensors do not create contacts: the pair query skips any pair
		// with a sensor shape (sensors are handled by the sensor system).
		w.updateBroadPhasePairs()

		tassert.Equal(t, 0, w.Counters().ContactCount)
		tassert.Equal(t, 0, getSetCount(&w.broadPhase.pairSet))
	})

	t.Run("SegmentVsSegmentCannotCollide", func(t *testing.T) {
		def := DefaultWorldDef()
		w := NewWorld(&def)
		defer w.Destroy()

		shapeDef := DefaultShapeDef()
		bodyDef := DefaultBodyDef()
		bodyDef.Type = DynamicBody
		segment := Segment{Point1: Vec2{X: -1.0, Y: 0.0}, Point2: Vec2{X: 1.0, Y: 0.0}}

		bodyDef.Position = Vec2Zero
		bodyA := w.CreateBody(&bodyDef)
		_ = w.CreateSegmentShape(bodyA, &shapeDef, &segment)

		bodyDef.Position = Vec2{X: 0.0, Y: 0.05}
		bodyB := w.CreateBody(&bodyDef)
		_ = w.CreateSegmentShape(bodyB, &shapeDef, &segment)

		w.updateBroadPhasePairs()

		// No manifold function for segment vs segment: no contact, no pair.
		tassert.Equal(t, 0, w.Counters().ContactCount)
		tassert.Equal(t, 0, getSetCount(&w.broadPhase.pairSet))
	})
}

func TestPromoteTouchingContactsGraphAndIslandMerge(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()

	// A-B and B-C overlap; A-C do not (including fat AABB margins).
	bodyIDA, _ := addDynamicCircle(w, 0.0, 0.0, &shapeDef)
	bodyIDB, _ := addDynamicCircle(w, 0.9, 0.0, &shapeDef)
	bodyIDC, _ := addDynamicCircle(w, 1.8, 0.0, &shapeDef)

	// Each awake body starts in its own island.
	require.Equal(t, 3, w.Counters().IslandCount)

	w.updateBroadPhasePairs()
	require.Equal(t, 2, w.Counters().ContactCount)

	promoteTouchingContacts(t, w)

	// The islands merged into one island holding both contacts.
	require.Equal(t, 1, w.Counters().IslandCount)

	bodyA := w.getBodyFullID(bodyIDA)
	bodyB := w.getBodyFullID(bodyIDB)
	bodyC := w.getBodyFullID(bodyIDC)
	require.NotEqual(t, NullIndex, bodyA.islandID)
	tassert.Equal(t, bodyA.islandID, bodyB.islandID)
	tassert.Equal(t, bodyA.islandID, bodyC.islandID)

	isl := &w.islands[bodyA.islandID]
	tassert.Equal(t, awakeSet, isl.setIndex)
	require.Len(t, isl.bodies, 3)
	require.Len(t, isl.contacts, 2)
	tassert.Empty(t, isl.joints)
	tassert.Equal(t, 0, isl.constraintRemoveCount)

	// Island body list walk is consistent with body bookkeeping.
	for i, bodyID := range isl.bodies {
		b := &w.bodies[bodyID]
		tassert.Equal(t, isl.islandID, b.islandID, "body %d", i)
		tassert.Equal(t, i, b.islandIndex, "body %d", i)
	}

	// Island contact list walk is consistent with contact bookkeeping, and
	// every contact lives in the constraint graph with sane indices.
	awake := &w.solverSets[awakeSet]
	tassert.Empty(t, awake.contactSims)
	for i, link := range isl.contacts {
		c := &w.contacts[link.contactID]
		tassert.Equal(t, isl.islandID, c.islandID, "contact %d", i)
		tassert.Equal(t, i, c.islandIndex, "contact %d", i)
		tassert.Equal(t, c.edges[0].bodyID, link.bodyIDA, "contact %d", i)
		tassert.Equal(t, c.edges[1].bodyID, link.bodyIDB, "contact %d", i)
		tassert.Equal(t, awakeSet, c.setIndex, "contact %d", i)
		tassert.NotEqual(t, uint32(0), c.flags&contactTouchingFlag, "contact %d", i)

		require.True(t, 0 <= c.colorIndex && c.colorIndex < GraphColorCount, "contact %d", i)
		color := &w.constraintGraph.colors[c.colorIndex]
		require.True(t, 0 <= c.localIndex && c.localIndex < len(color.contactSims), "contact %d", i)
		sim := &color.contactSims[c.localIndex]
		tassert.Equal(t, c.contactID, sim.contactID, "contact %d", i)
		tassert.NotEqual(t, uint32(0), sim.simFlags&simTouchingFlag, "contact %d", i)
		tassert.Positive(t, sim.manifold.PointCount, "contact %d", i)

		// Dynamic vs dynamic contacts occupy the color body sets.
		if c.colorIndex != overflowIndex {
			tassert.True(t, getBit(&color.bodySet, uint32(c.edges[0].bodyID)), "contact %d", i)
			tassert.True(t, getBit(&color.bodySet, uint32(c.edges[1].bodyID)), "contact %d", i)
		}

		// Body sim indices and inverse masses were captured from the awake set.
		tassert.Equal(t, w.bodies[c.edges[0].bodyID].localIndex, sim.bodySimIndexA, "contact %d", i)
		tassert.Equal(t, w.bodies[c.edges[1].bodyID].localIndex, sim.bodySimIndexB, "contact %d", i)
		tassert.Positive(t, sim.invMassA, "contact %d", i)
		tassert.Positive(t, sim.invMassB, "contact %d", i)
	}

	// The two contacts share bodyB so they must be in different colors.
	c0 := &w.contacts[isl.contacts[0].contactID]
	c1 := &w.contacts[isl.contacts[1].contactID]
	tassert.NotEqual(t, c0.colorIndex, c1.colorIndex)
}

func TestAddContactToGraphStaticBody(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()

	// Static ground overlapping a dynamic circle.
	bodyDef := DefaultBodyDef()
	bodyDef.Type = StaticBody
	bodyDef.Position = Vec2Zero
	groundID := w.CreateBody(&bodyDef)
	ground := MakeBox(2.0, 0.5)
	_ = w.CreatePolygonShape(groundID, &shapeDef, &ground)

	_, _ = addDynamicCircle(w, 0.0, 0.75, &shapeDef)

	w.updateBroadPhasePairs()
	require.Equal(t, 1, w.Counters().ContactCount)

	promoteTouchingContacts(t, w)

	c := &w.contacts[0]
	require.NotEqual(t, NullIndex, c.colorIndex)

	// Static constraint colors build from the end (below overflow).
	tassert.Equal(t, overflowIndex-1, c.colorIndex)

	color := &w.constraintGraph.colors[c.colorIndex]
	sim := &color.contactSims[c.localIndex]

	// The static body has no sim index and zero inverse mass.
	ground2 := w.getBodyFullID(groundID)
	if c.edges[0].bodyID == ground2.id {
		tassert.Equal(t, NullIndex, sim.bodySimIndexA)
		tassert.InDelta(t, 0.0, sim.invMassA, 0.0)
		tassert.InDelta(t, 0.0, sim.invIA, 0.0)
	} else {
		tassert.Equal(t, NullIndex, sim.bodySimIndexB)
		tassert.InDelta(t, 0.0, sim.invMassB, 0.0)
		tassert.InDelta(t, 0.0, sim.invIB, 0.0)
	}

	// Static bodies are not in islands; the island holds the one contact.
	tassert.Equal(t, NullIndex, ground2.islandID)
	tassert.Equal(t, 1, w.Counters().IslandCount)
	isl := &w.islands[c.islandID]
	require.Len(t, isl.bodies, 1)
	require.Len(t, isl.contacts, 1)
}

func TestUpdateContactTouchingMixingAndSeparation(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDefA := DefaultShapeDef()
	shapeDefA.Material.Friction = 0.4
	shapeDefA.Material.Restitution = 0.1
	shapeDefB := DefaultShapeDef()
	shapeDefB.Material.Friction = 0.9
	shapeDefB.Material.Restitution = 0.7
	shapeDefB.EnableHitEvents = true

	_, shapeIDA := addDynamicCircle(w, 0.0, 0.0, &shapeDefA)
	bodyIDB, shapeIDB := addDynamicCircle(w, 0.5, 0.0, &shapeDefB)

	w.updateBroadPhasePairs()
	require.Equal(t, 1, w.Counters().ContactCount)

	c := &w.contacts[0]
	sim := w.getContactSim(c)
	shapeA := w.getShape(shapeIDA)
	shapeB := w.getShape(shapeIDB)

	transformA := w.getBodyTransform(shapeA.bodyID)
	transformB := w.getBodyTransform(shapeB.bodyID)
	touching := w.updateContact(sim, shapeA, transformA, Vec2Zero, shapeB, transformB, Vec2Zero)

	require.True(t, touching)
	tassert.NotEqual(t, uint32(0), sim.simFlags&simTouchingFlag)
	tassert.Equal(t, 1, sim.manifold.PointCount) // circle vs circle: one point
	tassert.False(t, sim.manifold.Points[0].Persisted)

	// Default mixing: friction sqrt(fA*fB), restitution max(rA, rB). Use
	// float64 variables so the expectation rounds exactly like the callback.
	frictionA, frictionB := shapeDefA.Material.Friction, shapeDefB.Material.Friction
	tassert.InDelta(t, math.Sqrt(frictionA*frictionB), sim.friction, 0.0)
	tassert.InDelta(t, shapeDefB.Material.Restitution, sim.restitution, 0.0)

	// Hit events enabled on shape B while touching.
	tassert.NotEqual(t, uint32(0), sim.simFlags&simEnableHitEvent)

	// Warm-start impulse matching: a second update persists the point.
	sim.manifold.Points[0].NormalImpulse = 1.5
	sim.manifold.Points[0].TangentImpulse = -0.25
	touching = w.updateContact(sim, shapeA, transformA, Vec2Zero, shapeB, transformB, Vec2Zero)
	require.True(t, touching)
	require.Equal(t, 1, sim.manifold.PointCount)
	tassert.True(t, sim.manifold.Points[0].Persisted)
	tassert.InDelta(t, 1.5, sim.manifold.Points[0].NormalImpulse, 0.0)
	tassert.InDelta(t, -0.25, sim.manifold.Points[0].TangentImpulse, 0.0)

	// Move body B far away: touching clears along with the flags.
	w.SetBodyTransform(bodyIDB, Vec2{X: 10.0, Y: 0.0}, RotIdentity)
	transformB = w.getBodyTransform(shapeB.bodyID)
	touching = w.updateContact(sim, shapeA, transformA, Vec2Zero, shapeB, transformB, Vec2Zero)

	require.False(t, touching)
	tassert.Equal(t, uint32(0), sim.simFlags&simTouchingFlag)
	tassert.Equal(t, uint32(0), sim.simFlags&simEnableHitEvent)
	tassert.Equal(t, 0, sim.manifold.PointCount)
}

func TestUpdateContactBoxBoxTwoPoints(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()
	bodyDef := DefaultBodyDef()
	bodyDef.Type = DynamicBody
	box := MakeBox(0.5, 0.5)

	bodyDef.Position = Vec2Zero
	bodyA := w.CreateBody(&bodyDef)
	_ = w.CreatePolygonShape(bodyA, &shapeDef, &box)

	bodyDef.Position = Vec2{X: 0.0, Y: 0.9}
	bodyB := w.CreateBody(&bodyDef)
	_ = w.CreatePolygonShape(bodyB, &shapeDef, &box)

	w.updateBroadPhasePairs()
	require.Equal(t, 1, w.Counters().ContactCount)

	c := &w.contacts[0]
	sim := w.getContactSim(c)
	shapeA := &w.shapes[c.shapeIDA]
	shapeB := &w.shapes[c.shapeIDB]

	touching := w.updateContact(sim, shapeA, w.getBodyTransform(shapeA.bodyID), Vec2Zero,
		shapeB, w.getBodyTransform(shapeB.bodyID), Vec2Zero)

	require.True(t, touching)
	tassert.Equal(t, 2, sim.manifold.PointCount) // face contact: two points
}

func TestContactDataAccessors(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()
	bodyIDA, shapeIDA := addDynamicCircle(w, 0.0, 0.0, &shapeDef)
	_, shapeIDB := addDynamicCircle(w, 0.5, 0.0, &shapeDef)

	w.updateBroadPhasePairs()
	promoteTouchingContacts(t, w)

	require.Equal(t, 1, w.Counters().ContactCount)
	c := &w.contacts[0]

	// b2Contact_GetData
	fullID := ContactID{index1: int32(c.contactID + 1), world0: w.worldID, generation: c.generation}
	require.True(t, w.IsContactValid(fullID))
	data := w.ContactData(fullID)
	tassert.Equal(t, fullID, data.ContactID)
	tassert.Equal(t, shapeIDA, data.ShapeIDA)
	tassert.Equal(t, shapeIDB, data.ShapeIDB)
	tassert.Equal(t, 1, data.Manifold.PointCount)

	// b2Body_GetContactData
	capacity := w.BodyContactCapacity(bodyIDA)
	require.Equal(t, 1, capacity)
	buffer := make([]ContactData, capacity)
	n := w.BodyContactData(bodyIDA, buffer)
	require.Equal(t, 1, n)
	tassert.Equal(t, data, buffer[0])

	// b2Shape_GetContactData
	require.Equal(t, 1, w.ShapeContactCapacity(shapeIDB))
	buffer2 := make([]ContactData, 4)
	n = w.ShapeContactData(shapeIDB, buffer2)
	require.Equal(t, 1, n)
	tassert.Equal(t, data, buffer2[0])
}

func TestDemoteContactUnlinksIslandAndGraph(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()
	_, _ = addDynamicCircle(w, 0.0, 0.0, &shapeDef)
	bodyIDB, _ := addDynamicCircle(w, 0.5, 0.0, &shapeDef)

	w.updateBroadPhasePairs()
	promoteTouchingContacts(t, w)

	c := &w.contacts[0]
	islandID := c.islandID
	colorIndex := c.colorIndex
	require.NotEqual(t, NullIndex, islandID)
	require.NotEqual(t, NullIndex, colorIndex)
	colorLen := len(w.constraintGraph.colors[colorIndex].contactSims)

	// Separate the bodies and run the narrow phase: stopped touching.
	w.SetBodyTransform(bodyIDB, Vec2{X: 10.0, Y: 0.0}, RotIdentity)
	sim := w.getContactSim(c)
	shapeA := &w.shapes[c.shapeIDA]
	shapeB := &w.shapes[c.shapeIDB]
	touching := w.updateContact(sim, shapeA, w.getBodyTransform(shapeA.bodyID), Vec2Zero,
		shapeB, w.getBodyTransform(shapeB.bodyID), Vec2Zero)
	require.False(t, touching)

	demoteContact(w, c)

	// Contact is out of the island; the island tracks the removal.
	tassert.Equal(t, NullIndex, c.islandID)
	tassert.Equal(t, NullIndex, c.islandIndex)
	isl := &w.islands[islandID]
	tassert.Empty(t, isl.contacts)
	tassert.Equal(t, 1, isl.constraintRemoveCount)

	// Graph color released the sim and the body set bits.
	color := &w.constraintGraph.colors[colorIndex]
	tassert.Len(t, color.contactSims, colorLen-1)
	if colorIndex != overflowIndex {
		tassert.False(t, getBit(&color.bodySet, uint32(c.edges[0].bodyID)))
		tassert.False(t, getBit(&color.bodySet, uint32(c.edges[1].bodyID)))
	}

	// Contact sim is back in the awake set as non-touching.
	tassert.Equal(t, awakeSet, c.setIndex)
	tassert.Equal(t, NullIndex, c.colorIndex)
	awake := &w.solverSets[awakeSet]
	require.Len(t, awake.contactSims, 1)
	tassert.Equal(t, c.contactID, awake.contactSims[c.localIndex].contactID)
	tassert.Equal(t, 1, w.Counters().ContactCount)
}

func TestDestroyContactTouchingEmitsEndEventAndUnlinks(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()
	shapeDef.EnableContactEvents = true
	bodyIDA, shapeIDA := addDynamicCircle(w, 0.0, 0.0, &shapeDef)
	bodyIDB, shapeIDB := addDynamicCircle(w, 0.5, 0.0, &shapeDef)

	w.updateBroadPhasePairs()
	promoteTouchingContacts(t, w)

	c := &w.contacts[0]
	require.NotEqual(t, uint32(0), c.flags&contactEnableContactEvents)
	islandID := c.islandID
	colorIndex := c.colorIndex
	generation := c.generation
	pairKey := shapePairKey(uint32(c.shapeIDA), uint32(c.shapeIDB))

	w.destroyContact(c, true)

	// End touch event was emitted into the current end event buffer.
	events := w.contactEndEvents[w.endEventArrayIndex]
	require.Len(t, events, 1)
	tassert.Equal(t, shapeIDA, events[0].ShapeIDA)
	tassert.Equal(t, shapeIDB, events[0].ShapeIDB)
	tassert.Equal(t, int32(1), events[0].ContactID.index1)
	tassert.Equal(t, generation, events[0].ContactID.generation)

	// Island bookkeeping: contact removed, removal counted.
	isl := &w.islands[islandID]
	tassert.Empty(t, isl.contacts)
	tassert.Equal(t, 1, isl.constraintRemoveCount)

	// Graph entry removed.
	tassert.Empty(t, w.constraintGraph.colors[colorIndex].contactSims)

	// Body contact lists and counters updated; pair removed; id recycled.
	tassert.Equal(t, 0, w.getBodyFullID(bodyIDA).contactCount)
	tassert.Equal(t, 0, w.getBodyFullID(bodyIDB).contactCount)
	tassert.Equal(t, NullIndex, w.getBodyFullID(bodyIDA).headContactKey)
	tassert.Equal(t, NullIndex, w.getBodyFullID(bodyIDB).headContactKey)
	tassert.False(t, containsKey(&w.broadPhase.pairSet, pairKey))
	tassert.Equal(t, 0, w.Counters().ContactCount)
	tassert.Equal(t, NullIndex, c.contactID)
	tassert.Equal(t, NullIndex, c.setIndex)
	tassert.Equal(t, NullIndex, c.colorIndex)
	tassert.Equal(t, NullIndex, c.localIndex)
}

func TestDestroyBodyDestroysContacts(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()
	bodyIDA, _ := addDynamicCircle(w, 0.0, 0.0, &shapeDef)
	bodyIDB, _ := addDynamicCircle(w, 0.9, 0.0, &shapeDef)
	bodyIDC, _ := addDynamicCircle(w, 1.8, 0.0, &shapeDef)

	w.updateBroadPhasePairs()
	promoteTouchingContacts(t, w)
	require.Equal(t, 2, w.Counters().ContactCount)

	// Destroying the middle body destroys both contacts end-to-end.
	w.DestroyBody(bodyIDB)

	tassert.Equal(t, 0, w.Counters().ContactCount)
	tassert.Equal(t, 0, w.getBodyFullID(bodyIDA).contactCount)
	tassert.Equal(t, 0, w.getBodyFullID(bodyIDC).contactCount)
	tassert.Equal(t, NullIndex, w.getBodyFullID(bodyIDA).headContactKey)
	tassert.Equal(t, NullIndex, w.getBodyFullID(bodyIDC).headContactKey)
	tassert.Equal(t, 0, getSetCount(&w.broadPhase.pairSet))

	// All graph colors are empty again.
	for i := range GraphColorCount {
		tassert.Empty(t, w.constraintGraph.colors[i].contactSims, "color %d", i)
	}

	// The merged island lost body B and both contacts but stays consistent.
	bodyA := w.getBodyFullID(bodyIDA)
	require.NotEqual(t, NullIndex, bodyA.islandID)
	isl := &w.islands[bodyA.islandID]
	tassert.Empty(t, isl.contacts)
	tassert.Equal(t, 2, isl.constraintRemoveCount)
	for i, bodyID := range isl.bodies {
		tassert.Equal(t, i, w.bodies[bodyID].islandIndex)
	}
}

func TestSleepWakeMovesTouchingContactThroughGraph(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()
	bodyIDA, _ := addDynamicCircle(w, 0.0, 0.0, &shapeDef)
	_, _ = addDynamicCircle(w, 0.5, 0.0, &shapeDef)

	w.updateBroadPhasePairs()
	promoteTouchingContacts(t, w)

	c := &w.contacts[0]
	bodyA := w.getBodyFullID(bodyIDA)
	islandID := bodyA.islandID
	require.NotEqual(t, NullIndex, c.colorIndex)

	// Put the island to sleep: the touching contact moves from the graph to
	// the sleeping set.
	w.trySleepIsland(islandID)

	sleepSetID := bodyA.setIndex
	require.GreaterOrEqual(t, sleepSetID, firstSleepingSet)
	tassert.Equal(t, sleepSetID, c.setIndex)
	tassert.Equal(t, NullIndex, c.colorIndex)
	sleepSet := &w.solverSets[sleepSetID]
	require.Len(t, sleepSet.contactSims, 1)
	tassert.Equal(t, c.contactID, sleepSet.contactSims[c.localIndex].contactID)

	// Waking the set pushes the touching contact back into the graph
	// (exercises addContactToGraph through the wakeSolverSet call site).
	w.wakeSolverSet(sleepSetID)

	tassert.Equal(t, awakeSet, c.setIndex)
	require.NotEqual(t, NullIndex, c.colorIndex)
	color := &w.constraintGraph.colors[c.colorIndex]
	tassert.Equal(t, c.contactID, color.contactSims[c.localIndex].contactID)
	tassert.Equal(t, awakeSet, w.islands[islandID].setIndex)
}

func TestDestroyContactWakesSleepingBodies(t *testing.T) {
	def := DefaultWorldDef()
	w := NewWorld(&def)
	defer w.Destroy()

	shapeDef := DefaultShapeDef()
	bodyIDA, _ := addDynamicCircle(w, 0.0, 0.0, &shapeDef)
	bodyIDB, _ := addDynamicCircle(w, 0.5, 0.0, &shapeDef)

	w.updateBroadPhasePairs()
	promoteTouchingContacts(t, w)

	bodyA := w.getBodyFullID(bodyIDA)
	w.trySleepIsland(bodyA.islandID)
	require.GreaterOrEqual(t, bodyA.setIndex, firstSleepingSet)

	// Destroy the touching contact of a sleeping island with wakeBodies=true:
	// the sleeping-set removal path runs and both bodies wake.
	c := &w.contacts[0]
	w.destroyContact(c, true)

	tassert.Equal(t, awakeSet, w.getBodyFullID(bodyIDA).setIndex)
	tassert.Equal(t, awakeSet, w.getBodyFullID(bodyIDB).setIndex)
	tassert.Equal(t, 0, w.Counters().ContactCount)
}

// buildDeterminismScene creates 10 bodies in overlapping clusters: three
// dynamic circle clusters and a static ground overlapping the first cluster.
func buildDeterminismScene(w *World) {
	shapeDef := DefaultShapeDef()
	shapeDef.EnableContactEvents = true

	bodyDef := DefaultBodyDef()
	bodyDef.Type = StaticBody
	bodyDef.Position = Vec2{X: 0.0, Y: -0.75}
	groundID := w.CreateBody(&bodyDef)
	ground := MakeBox(3.0, 0.5)
	_ = w.CreatePolygonShape(groundID, &shapeDef, &ground)

	positions := []Vec2{
		// cluster 1 (overlaps the ground)
		{X: 0.0, Y: 0.0}, {X: 0.6, Y: 0.1}, {X: 0.3, Y: 0.5},
		// cluster 2
		{X: 10.0, Y: 0.0}, {X: 10.5, Y: 0.0}, {X: 10.25, Y: 0.4}, {X: 11.0, Y: 0.0},
		// cluster 3
		{X: 20.0, Y: 0.0}, {X: 20.4, Y: 0.3},
	}

	for _, p := range positions {
		_, _ = addDynamicCircle(w, p.X, p.Y, &shapeDef)
	}
}

func TestDeterminismTwoWorldsIdenticalState(t *testing.T) {
	def1 := DefaultWorldDef()
	w1 := NewWorld(&def1)
	defer w1.Destroy()
	def2 := DefaultWorldDef()
	w2 := NewWorld(&def2)
	defer w2.Destroy()

	buildDeterminismScene(w1)
	buildDeterminismScene(w2)

	w1.updateBroadPhasePairs()
	w2.updateBroadPhasePairs()
	promoteTouchingContacts(t, w1)
	promoteTouchingContacts(t, w2)

	// Something interesting happened.
	require.Positive(t, w1.Counters().ContactCount)
	require.Positive(t, w1.Counters().IslandCount)

	// Contacts are field-identical, including edges, flags, islands, and
	// graph slots.
	require.Equal(t, w1.contacts, w2.contacts)

	// Island arrays (bodies, contact links, joint links) are identical.
	require.Equal(t, w1.islands, w2.islands)

	// Bodies carry identical island/contact bookkeeping.
	require.Equal(t, w1.bodies, w2.bodies)

	// Constraint graph colors hold identical sims in identical order.
	for i := range GraphColorCount {
		require.Equal(t, w1.constraintGraph.colors[i].contactSims,
			w2.constraintGraph.colors[i].contactSims, "color %d", i)
	}

	// Solver sets match: same non-touching contacts and islands.
	require.Len(t, w2.solverSets, len(w1.solverSets))
	for i := range w1.solverSets {
		require.Equal(t, w1.solverSets[i].contactSims, w2.solverSets[i].contactSims, "set %d", i)
		require.Equal(t, w1.solverSets[i].islandSims, w2.solverSets[i].islandSims, "set %d", i)
	}

	// Counters agree.
	tassert.Equal(t, w1.Counters(), w2.Counters())
}
