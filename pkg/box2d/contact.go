// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/contact.h, src/contact.c.
//
// Contacts and determinism (upstream comment):
// A deterministic simulation requires contacts to exist in the same order in
// b2Island no matter the thread count. The order must reproduce from run to
// run. This is necessary because the Gauss-Seidel constraint solver is order
// dependent.
//
// Creation:
//   - Contacts are created using results from updateBroadPhasePairs
//   - These results are ordered according to the order of the broad-phase
//     move array
//   - The move array is ordered according to the shape creation order using
//     a bitset
//   - The island/shape/body order is determined by creation order
//   - Logically contacts are only created for awake bodies, so they are
//     immediately added to the awake contact array (serially)
//
// Island linking:
//   - The awake contact array is built from the body-contact graph for all
//     awake bodies in awake islands.
//   - Awake contacts are solved in parallel and they generate contact state
//     changes.
//   - These state changes may link islands together using union find.
//   - The state changes are ordered using a bit array that encompasses all
//     contacts
//   - As long as contacts are created in deterministic order, island link
//     order is deterministic.
//   - This keeps the order of contacts in islands deterministic
//
// Deviations from upstream:
//   - The B2_ENABLE_VALIDATION-only fields bodyIdA/bodyIdB on b2ContactSim
//     are not ported (validation is compiled out in this port, see core.go).
//   - The s_registers table guarded by s_initialized becomes the package-level
//     immutable contactRegisters array literal below; b2InitializeContactRegisters
//     is kept as a no-op for call-site parity.

package box2d

// Contact flags (upstream enum b2ContactFlags).
const (
	// contactTouchingFlag is set when the solid shapes are touching.
	contactTouchingFlag uint32 = 0x00000001

	// contactHitEventFlag means the contact has a hit event.
	//nolint:unused // upstream b2ContactFlags parity; hit events use simEnableHitEvent in this port
	contactHitEventFlag uint32 = 0x00000002

	// contactEnableContactEvents means this contact wants contact events.
	contactEnableContactEvents uint32 = 0x00000004
)

// contactEdge connects bodies and contacts together in a contact graph where
// each body is a node and each contact is an edge (upstream b2ContactEdge).
// A contact edge belongs to a doubly linked list maintained in each attached
// body. Each contact has two contact edges, one for each attached body.
type contactEdge struct {
	bodyID  int
	prevKey int
	nextKey int
}

// contact is cold contact data used as a persistent handle and for persistent
// island connectivity (upstream b2Contact).
type contact struct {
	edges [2]contactEdge

	// A contact only belongs to an island if touching, otherwise NullIndex.
	islandID int

	// Index into the island's contacts array for O(1) swap-removal.
	// NullIndex when not in an island.
	islandIndex int

	// index of simulation set stored in World; NullIndex when the slot is free
	setIndex int

	// index into the constraint graph color array; NullIndex for non-touching
	// or sleeping contacts and when the slot is free
	colorIndex int

	// contact index within set or graph color; NullIndex when the slot is free
	localIndex int

	shapeIDA  int
	shapeIDB  int
	contactID int

	// contact flags (contactTouchingFlag, ...)
	flags uint32

	// This is monotonically advanced when a contact is allocated in this slot.
	// Used to check for invalid ContactID.
	generation uint32
}

// Contact sim flags, shifted to be distinct from the contact flags
// (upstream enum b2ContactSimFlags).
const (
	// simTouchingFlag is set when the shapes are touching.
	simTouchingFlag uint32 = 0x00010000

	// simDisjoint means this contact no longer has overlapping AABBs.
	simDisjoint uint32 = 0x00020000

	// simStartedTouching means this contact started touching.
	simStartedTouching uint32 = 0x00040000

	// simStoppedTouching means this contact stopped touching.
	simStoppedTouching uint32 = 0x00080000

	// simEnableHitEvent means this contact has a hit event.
	simEnableHitEvent uint32 = 0x00100000

	// simEnablePreSolveEvents means this contact wants pre-solve events.
	simEnablePreSolveEvents uint32 = 0x00200000

	// simRelativeTransformValid means this contact has a cached relative transform.
	simRelativeTransformValid uint32 = 0x00400000
)

// contactSim manages contact between two shapes (upstream b2ContactSim).
// A contact exists for each overlapping AABB in the broad-phase (except if
// filtered). Therefore a contact object may exist that has no contact points.
type contactSim struct {
	contactID int

	cachedTransformA Transform
	cachedTransformB Transform

	// Transient body indices
	bodySimIndexA int
	bodySimIndexB int

	shapeIDA int
	shapeIDB int

	invMassA float64
	invIA    float64

	invMassB float64
	invIB    float64

	manifold Manifold

	// Mixed friction and restitution
	friction          float64
	restitution       float64
	rollingResistance float64
	tangentSpeed      float64

	// contact sim flags (simTouchingFlag, ...)
	simFlags uint32

	cache SimplexCache
}

// getContactFullID returns a validated contact for a full contact id
// (upstream static b2GetContactFullId).
func (w *World) getContactFullID(contactID ContactID) *contact {
	id := int(contactID.index1) - 1
	c := &w.contacts[id]
	assert(c.contactID == id && c.generation == contactID.generation)
	return c
}

// ContactData returns the contact data for a contact id
// (upstream b2Contact_GetData).
func (w *World) ContactData(contactID ContactID) ContactData {
	c := w.getContactFullID(contactID)
	contactSim := w.getContactSim(c)
	shapeA := &w.shapes[c.shapeIDA]
	shapeB := &w.shapes[c.shapeIDB]

	data := ContactData{
		ContactID: contactID,
		ShapeIDA: ShapeID{
			index1:     int32(shapeA.id + 1),
			world0:     contactID.world0,
			generation: shapeA.generation,
		},
		ShapeIDB: ShapeID{
			index1:     int32(shapeB.id + 1),
			world0:     contactID.world0,
			generation: shapeB.generation,
		},
		Manifold: contactSim.manifold,
	}

	return data
}

// manifoldFcn computes the manifold between two shapes
// (upstream typedef b2ManifoldFcn).
//
// Manifold functions should compute important results in local space to
// improve precision. However, this interface function takes two world
// transforms instead of a relative transform for these reasons:
//
// First:
// The anchors need to be computed relative to the shape origin in world
// space. This is necessary so the solver does not need to access static body
// transforms. Not even in constraint preparation. This approach has world
// space vectors yet retains precision.
//
// Second:
// ManifoldPoint.ClipPoint is very useful for debugging and it is in world
// space.
//
// Third:
// The user may call the manifold functions directly and they should be easy
// to use and have easy to use results.
type manifoldFcn func(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold

// contactRegister is one entry of the collide dispatch table
// (upstream struct b2ContactRegister).
type contactRegister struct {
	fcn     manifoldFcn
	primary bool
}

// circleManifold mirrors static b2CircleManifold.
func circleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollideCircles(&shapeA.circle, xfA, &shapeB.circle, xfB)
}

// capsuleAndCircleManifold mirrors static b2CapsuleAndCircleManifold.
func capsuleAndCircleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollideCapsuleAndCircle(&shapeA.capsule, xfA, &shapeB.circle, xfB)
}

// capsuleManifold mirrors static b2CapsuleManifold.
func capsuleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollideCapsules(&shapeA.capsule, xfA, &shapeB.capsule, xfB)
}

// polygonAndCircleManifold mirrors static b2PolygonAndCircleManifold.
func polygonAndCircleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollidePolygonAndCircle(&shapeA.polygon, xfA, &shapeB.circle, xfB)
}

// polygonAndCapsuleManifold mirrors static b2PolygonAndCapsuleManifold.
func polygonAndCapsuleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollidePolygonAndCapsule(&shapeA.polygon, xfA, &shapeB.capsule, xfB)
}

// polygonManifold mirrors static b2PolygonManifold.
func polygonManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollidePolygons(&shapeA.polygon, xfA, &shapeB.polygon, xfB)
}

// segmentAndCircleManifold mirrors static b2SegmentAndCircleManifold.
func segmentAndCircleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollideSegmentAndCircle(&shapeA.segment, xfA, &shapeB.circle, xfB)
}

// segmentAndCapsuleManifold mirrors static b2SegmentAndCapsuleManifold.
func segmentAndCapsuleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollideSegmentAndCapsule(&shapeA.segment, xfA, &shapeB.capsule, xfB)
}

// segmentAndPolygonManifold mirrors static b2SegmentAndPolygonManifold.
func segmentAndPolygonManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollideSegmentAndPolygon(&shapeA.segment, xfA, &shapeB.polygon, xfB)
}

// chainSegmentAndCircleManifold mirrors static b2ChainSegmentAndCircleManifold.
func chainSegmentAndCircleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	_ = cache
	return CollideChainSegmentAndCircle(&shapeA.chainSegment, xfA, &shapeB.circle, xfB)
}

// chainSegmentAndCapsuleManifold mirrors static b2ChainSegmentAndCapsuleManifold.
func chainSegmentAndCapsuleManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	return CollideChainSegmentAndCapsule(&shapeA.chainSegment, xfA, &shapeB.capsule, xfB, cache)
}

// chainSegmentAndPolygonManifold mirrors static b2ChainSegmentAndPolygonManifold.
func chainSegmentAndPolygonManifold(shapeA *shape, xfA Transform, shapeB *shape, xfB Transform, cache *SimplexCache) Manifold {
	return CollideChainSegmentAndPolygon(&shapeA.chainSegment, xfA, &shapeB.polygon, xfB, cache)
}

// contactRegisters is the collide dispatch table indexed by shape type pair
// (upstream static s_registers filled by b2AddType calls in
// b2InitializeContactRegisters). Deviation from upstream: the table is a
// compile-time literal of pure functions instead of lazily initialized
// mutable state; entries mirror the upstream b2AddType(fcn, type1, type2)
// calls, with [type1][type2] primary and the mirrored [type2][type1] entry
// non-primary. Missing entries (e.g. segment vs segment) have a nil fcn and
// never collide.
var contactRegisters = [ShapeTypeCount][ShapeTypeCount]contactRegister{
	CircleShape: {
		CircleShape:       {fcn: circleManifold, primary: true},
		CapsuleShape:      {fcn: capsuleAndCircleManifold, primary: false},
		PolygonShape:      {fcn: polygonAndCircleManifold, primary: false},
		SegmentShape:      {fcn: segmentAndCircleManifold, primary: false},
		ChainSegmentShape: {fcn: chainSegmentAndCircleManifold, primary: false},
	},
	CapsuleShape: {
		CircleShape:       {fcn: capsuleAndCircleManifold, primary: true},
		CapsuleShape:      {fcn: capsuleManifold, primary: true},
		PolygonShape:      {fcn: polygonAndCapsuleManifold, primary: false},
		SegmentShape:      {fcn: segmentAndCapsuleManifold, primary: false},
		ChainSegmentShape: {fcn: chainSegmentAndCapsuleManifold, primary: false},
	},
	PolygonShape: {
		CircleShape:       {fcn: polygonAndCircleManifold, primary: true},
		CapsuleShape:      {fcn: polygonAndCapsuleManifold, primary: true},
		PolygonShape:      {fcn: polygonManifold, primary: true},
		SegmentShape:      {fcn: segmentAndPolygonManifold, primary: false},
		ChainSegmentShape: {fcn: chainSegmentAndPolygonManifold, primary: false},
	},
	SegmentShape: {
		CircleShape:  {fcn: segmentAndCircleManifold, primary: true},
		CapsuleShape: {fcn: segmentAndCapsuleManifold, primary: true},
		PolygonShape: {fcn: segmentAndPolygonManifold, primary: true},
	},
	ChainSegmentShape: {
		CircleShape:  {fcn: chainSegmentAndCircleManifold, primary: true},
		CapsuleShape: {fcn: chainSegmentAndCapsuleManifold, primary: true},
		PolygonShape: {fcn: chainSegmentAndPolygonManifold, primary: true},
	},
}

// initializeContactRegisters mirrors b2InitializeContactRegisters. Deviation
// from upstream: the register table is the package-level immutable
// contactRegisters literal above, so this is a no-op kept for call-site
// parity with b2CreateWorld.
func initializeContactRegisters() {
}

// canCollide reports whether the two shape types have a collide function
// (upstream b2CanCollide).
func canCollide(typeA, typeB ShapeType) bool {
	return contactRegisters[typeA][typeB].fcn != nil
}

// createContact mirrors b2CreateContact.
//
// WARNING: this should never fail to create a contact because the pair
// already exists in the pairSet.
func (w *World) createContact(shapeA, shapeB *shape) {
	type1 := shapeA.shapeType
	type2 := shapeB.shapeType

	assert(0 <= type1 && type1 < ShapeTypeCount)
	assert(0 <= type2 && type2 < ShapeTypeCount)

	if contactRegisters[type1][type2].fcn == nil {
		// For example, no segment vs segment collision
		return
	}

	if !contactRegisters[type1][type2].primary {
		// flip order
		w.createContact(shapeB, shapeA)
		return
	}

	bodyA := &w.bodies[shapeA.bodyID]
	bodyB := &w.bodies[shapeB.bodyID]

	assert(bodyA.setIndex != disabledSet && bodyB.setIndex != disabledSet)
	assert(bodyA.setIndex != staticSet || bodyB.setIndex != staticSet)

	var setIndex int
	if bodyA.setIndex == awakeSet || bodyB.setIndex == awakeSet {
		setIndex = awakeSet
	} else {
		// sleeping and non-touching contacts live in the disabled set
		// later if this set is found to be touching then the sleeping
		// islands will be linked and the contact moved to the merged island
		setIndex = disabledSet
	}

	set := &w.solverSets[setIndex]

	// Create contact key and contact
	contactID := allocID(&w.contactIDPool)
	if contactID == len(w.contacts) {
		w.contacts = append(w.contacts, contact{})
	}

	shapeIDA := shapeA.id
	shapeIDB := shapeB.id

	c := &w.contacts[contactID]
	c.contactID = contactID
	c.generation++
	c.setIndex = setIndex
	c.colorIndex = NullIndex
	c.localIndex = len(set.contactSims)
	c.islandID = NullIndex
	c.islandIndex = NullIndex
	c.shapeIDA = shapeIDA
	c.shapeIDB = shapeIDB
	c.flags = 0

	assert(shapeA.sensorIndex == NullIndex && shapeB.sensorIndex == NullIndex)

	if shapeA.enableContactEvents || shapeB.enableContactEvents {
		c.flags |= contactEnableContactEvents
	}

	// Connect to body A
	{
		c.edges[0].bodyID = shapeA.bodyID
		c.edges[0].prevKey = NullIndex
		c.edges[0].nextKey = bodyA.headContactKey

		keyA := contactID << 1 // | 0 for edge index 0
		headContactKey := bodyA.headContactKey
		if headContactKey != NullIndex {
			headContact := &w.contacts[headContactKey>>1]
			headContact.edges[headContactKey&1].prevKey = keyA
		}
		bodyA.headContactKey = keyA
		bodyA.contactCount++
	}

	// Connect to body B
	{
		c.edges[1].bodyID = shapeB.bodyID
		c.edges[1].prevKey = NullIndex
		c.edges[1].nextKey = bodyB.headContactKey

		keyB := (contactID << 1) | 1
		headContactKey := bodyB.headContactKey
		if bodyB.headContactKey != NullIndex {
			headContact := &w.contacts[headContactKey>>1]
			headContact.edges[headContactKey&1].prevKey = keyB
		}
		bodyB.headContactKey = keyB
		bodyB.contactCount++
	}

	// Add to pair set for fast lookup.
	pairKey := shapePairKey(uint32(shapeIDA), uint32(shapeIDB))
	_ = addKey(&w.broadPhase.pairSet, pairKey)

	// Contacts are created as non-touching. Later if they are found to be
	// touching they will link islands and be moved into the constraint graph.
	set.contactSims = append(set.contactSims, contactSim{})
	contactSim := &set.contactSims[len(set.contactSims)-1]
	contactSim.contactID = contactID

	contactSim.bodySimIndexA = NullIndex
	contactSim.bodySimIndexB = NullIndex
	contactSim.invMassA = 0.0
	contactSim.invIA = 0.0
	contactSim.invMassB = 0.0
	contactSim.invIB = 0.0
	contactSim.shapeIDA = shapeIDA
	contactSim.shapeIDB = shapeIDB
	contactSim.cache = SimplexCache{}
	contactSim.manifold = Manifold{}

	// These get updated in the narrow phase, but these are needed for first touch
	contactSim.friction = w.frictionCallback(shapeA.material.Friction, shapeA.material.UserMaterialID,
		shapeB.material.Friction, shapeB.material.UserMaterialID)
	contactSim.restitution = w.restitutionCallback(shapeA.material.Restitution, shapeA.material.UserMaterialID,
		shapeB.material.Restitution, shapeB.material.UserMaterialID)

	contactSim.tangentSpeed = 0.0
	contactSim.simFlags = 0

	if shapeA.enablePreSolveEvents || shapeB.enablePreSolveEvents {
		contactSim.simFlags |= simEnablePreSolveEvents
	}
}

// destroyContact mirrors b2DestroyContact.
//
// A contact is destroyed when:
//   - broad-phase proxies stop overlapping
//   - a body is destroyed
//   - a body is disabled
//   - a body changes type from dynamic to kinematic or static
//   - a shape is destroyed
//   - contact filtering is modified
func (w *World) destroyContact(c *contact, wakeBodies bool) {
	// Remove pair from set
	pairKey := shapePairKey(uint32(c.shapeIDA), uint32(c.shapeIDB))
	_ = removeKey(&w.broadPhase.pairSet, pairKey)

	edgeA := &c.edges[0]
	edgeB := &c.edges[1]

	bodyIDA := edgeA.bodyID
	bodyIDB := edgeB.bodyID
	bodyA := &w.bodies[bodyIDA]
	bodyB := &w.bodies[bodyIDB]

	flags := c.flags
	touching := flags&contactTouchingFlag != 0

	// End touch event
	if touching && flags&contactEnableContactEvents != 0 {
		worldID := w.worldID
		shapeA := &w.shapes[c.shapeIDA]
		shapeB := &w.shapes[c.shapeIDB]
		shapeIDA := ShapeID{index1: int32(shapeA.id + 1), world0: worldID, generation: shapeA.generation}
		shapeIDB := ShapeID{index1: int32(shapeB.id + 1), world0: worldID, generation: shapeB.generation}

		contactID := ContactID{
			index1:     int32(c.contactID + 1),
			world0:     w.worldID,
			padding:    0,
			generation: c.generation,
		}

		event := ContactEndTouchEvent{
			ShapeIDA:  shapeIDA,
			ShapeIDB:  shapeIDB,
			ContactID: contactID,
		}

		w.contactEndEvents[w.endEventArrayIndex] = append(w.contactEndEvents[w.endEventArrayIndex], event)
	}

	// Remove from body A
	if edgeA.prevKey != NullIndex {
		prevContact := &w.contacts[edgeA.prevKey>>1]
		prevEdge := &prevContact.edges[edgeA.prevKey&1]
		prevEdge.nextKey = edgeA.nextKey
	}

	if edgeA.nextKey != NullIndex {
		nextContact := &w.contacts[edgeA.nextKey>>1]
		nextEdge := &nextContact.edges[edgeA.nextKey&1]
		nextEdge.prevKey = edgeA.prevKey
	}

	contactID := c.contactID

	edgeKeyA := contactID << 1 // | 0 for edge index 0
	if bodyA.headContactKey == edgeKeyA {
		bodyA.headContactKey = edgeA.nextKey
	}

	bodyA.contactCount--

	// Remove from body B
	if edgeB.prevKey != NullIndex {
		prevContact := &w.contacts[edgeB.prevKey>>1]
		prevEdge := &prevContact.edges[edgeB.prevKey&1]
		prevEdge.nextKey = edgeB.nextKey
	}

	if edgeB.nextKey != NullIndex {
		nextContact := &w.contacts[edgeB.nextKey>>1]
		nextEdge := &nextContact.edges[edgeB.nextKey&1]
		nextEdge.prevKey = edgeB.prevKey
	}

	edgeKeyB := (contactID << 1) | 1
	if bodyB.headContactKey == edgeKeyB {
		bodyB.headContactKey = edgeB.nextKey
	}

	bodyB.contactCount--

	// Remove contact from the array that owns it
	if c.islandID != NullIndex {
		w.unlinkContact(c)
	}

	if c.colorIndex != NullIndex {
		// contact is an active constraint
		assert(c.setIndex == awakeSet)
		w.removeContactFromGraph(bodyIDA, bodyIDB, c.colorIndex, c.localIndex)
	} else {
		// contact is non-touching or is sleeping
		assert(c.setIndex != awakeSet || c.flags&contactTouchingFlag == 0)
		set := &w.solverSets[c.setIndex]

		movedIndex := removeSwap(&set.contactSims, c.localIndex)
		if movedIndex != NullIndex {
			movedContactSim := &set.contactSims[c.localIndex]
			movedContact := &w.contacts[movedContactSim.contactID]
			movedContact.localIndex = c.localIndex
		}
	}

	// Free contact and id (preserve generation)
	c.contactID = NullIndex
	c.setIndex = NullIndex
	c.colorIndex = NullIndex
	c.localIndex = NullIndex
	freeID(&w.contactIDPool, contactID)

	if wakeBodies && touching {
		w.wakeBody(bodyA)
		w.wakeBody(bodyB)
	}
}

// getContactSim returns the contact sim for a contact, looking in the
// constraint graph for awake colored contacts (upstream b2GetContactSim).
func (w *World) getContactSim(c *contact) *contactSim {
	if c.setIndex == awakeSet && c.colorIndex != NullIndex {
		// contact lives in constraint graph
		assert(0 <= c.colorIndex && c.colorIndex < GraphColorCount)
		color := &w.constraintGraph.colors[c.colorIndex]
		return &color.contactSims[c.localIndex]
	}

	set := &w.solverSets[c.setIndex]
	return &set.contactSims[c.localIndex]
}

// updateContact updates the contact manifold and touching status and reports
// whether the shapes are touching (upstream b2UpdateContact).
// Note: do not assume the shape AABBs are overlapping or are valid.
func (w *World) updateContact(contactSim *contactSim, shapeA *shape, transformA Transform, centerOffsetA Vec2,
	shapeB *shape, transformB Transform, centerOffsetB Vec2,
) bool {
	// Save old manifold
	oldManifold := contactSim.manifold

	// Compute new manifold
	fcn := contactRegisters[shapeA.shapeType][shapeB.shapeType].fcn
	contactSim.manifold = fcn(shapeA, transformA, shapeB, transformB, &contactSim.cache)

	// Keep these updated in case the values on the shapes are modified
	contactSim.friction = w.frictionCallback(shapeA.material.Friction, shapeA.material.UserMaterialID,
		shapeB.material.Friction, shapeB.material.UserMaterialID)
	contactSim.restitution = w.restitutionCallback(shapeA.material.Restitution, shapeA.material.UserMaterialID,
		shapeB.material.Restitution, shapeB.material.UserMaterialID)

	if shapeA.material.RollingResistance > 0.0 || shapeB.material.RollingResistance > 0.0 {
		radiusA := getShapeRadius(shapeA)
		radiusB := getShapeRadius(shapeB)
		maxRadius := maxFloat(radiusA, radiusB)
		contactSim.rollingResistance = maxFloat(shapeA.material.RollingResistance, shapeB.material.RollingResistance) * maxRadius
	} else {
		contactSim.rollingResistance = 0.0
	}

	contactSim.tangentSpeed = shapeA.material.TangentSpeed + shapeB.material.TangentSpeed

	pointCount := contactSim.manifold.PointCount
	touching := pointCount > 0

	if touching && w.preSolveFcn != nil && contactSim.simFlags&simEnablePreSolveEvents != 0 {
		shapeIDA := ShapeID{index1: int32(shapeA.id + 1), world0: w.worldID, generation: shapeA.generation}
		shapeIDB := ShapeID{index1: int32(shapeB.id + 1), world0: w.worldID, generation: shapeB.generation}

		manifold := &contactSim.manifold
		bestSeparation := manifold.Points[0].Separation
		bestPoint := manifold.Points[0].ClipPoint

		// Get deepest point
		for i := 1; i < manifold.PointCount; i++ {
			separation := manifold.Points[i].Separation
			if separation < bestSeparation {
				bestSeparation = separation
				bestPoint = manifold.Points[i].ClipPoint
			}
		}

		// this call assumes thread safety
		touching = w.preSolveFcn(shapeIDA, shapeIDB, bestPoint, manifold.Normal, w.preSolveContext)
		if !touching {
			// disable contact
			pointCount = 0
			manifold.PointCount = 0
		}
	}

	// This flag is for testing
	if !w.enableSpeculative && pointCount == 2 {
		switch {
		case contactSim.manifold.Points[0].Separation > 1.5*LinearSlop:
			contactSim.manifold.Points[0] = contactSim.manifold.Points[1]
			contactSim.manifold.PointCount = 1
		// Faithful transliteration: upstream v3.2.0 repeats the Points[0]
		// condition here (likely intending Points[1]), making this branch
		// unreachable. Kept for parity.
		case contactSim.manifold.Points[0].Separation > 1.5*LinearSlop: //nolint:gocritic // dupCase: upstream duplicate, see above.
			contactSim.manifold.PointCount = 1
		default:
		}

		pointCount = contactSim.manifold.PointCount
	}

	if touching && (shapeA.enableHitEvents || shapeB.enableHitEvents) {
		contactSim.simFlags |= simEnableHitEvent
	} else {
		contactSim.simFlags &^= simEnableHitEvent
	}

	if pointCount > 0 {
		contactSim.manifold.RollingImpulse = oldManifold.RollingImpulse
	}

	// Match old contact ids to new contact ids and copy the
	// stored impulses to warm start the solver.
	unmatchedCount := 0
	for i := range pointCount {
		mp2 := &contactSim.manifold.Points[i]

		// shift anchors to be center of mass relative
		mp2.AnchorA = Sub(mp2.AnchorA, centerOffsetA)
		mp2.AnchorB = Sub(mp2.AnchorB, centerOffsetB)

		mp2.NormalImpulse = 0.0
		mp2.TangentImpulse = 0.0
		mp2.TotalNormalImpulse = 0.0
		mp2.NormalVelocity = 0.0
		mp2.Persisted = false

		id2 := mp2.ID

		for j := range oldManifold.PointCount {
			mp1 := &oldManifold.Points[j]

			if mp1.ID == id2 {
				mp2.NormalImpulse = mp1.NormalImpulse
				mp2.TangentImpulse = mp1.TangentImpulse
				mp2.Persisted = true

				// clear old impulse
				mp1.NormalImpulse = 0.0
				mp1.TangentImpulse = 0.0
				break
			}
		}

		if !mp2.Persisted {
			unmatchedCount++
		}
	}

	// B2_UNUSED(unmatchedCount): upstream keeps this for a disabled
	// experiment that redistributes unmatched impulses.
	_ = unmatchedCount

	if touching {
		contactSim.simFlags |= simTouchingFlag
	} else {
		contactSim.simFlags &^= simTouchingFlag
	}

	return touching
}
