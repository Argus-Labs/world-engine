// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/body.c, src/body.h.
//
// Public API mapping: b2Body_GetPosition → (*World).BodyPosition,
// b2Body_SetTransform → (*World).SetBodyTransform, b2Body_ApplyForce →
// (*World).ApplyBodyForce, and so on (getters are Get-less).
//
// Deviations from upstream:
//   - The fixed char name[32] becomes a Go string truncated to 31 bytes.
//   - b2Body_GetWorld is not ported: there is no world registry, callers
//     already hold the *World (see world.go).

package box2d

import "math"

// nameLength is the length of the body debug name buffer including the NUL
// terminator (upstream B2_NAME_LENGTH). Go strings keep up to nameLength-1
// bytes.
const nameLength = 32

// Body flags (upstream enum b2BodyFlags).
const (
	// lockLinearX means this body has fixed translation along the x-axis.
	lockLinearX uint32 = 0x00000001

	// lockLinearY means this body has fixed translation along the y-axis.
	lockLinearY uint32 = 0x00000002

	// lockAngularZ means this body has fixed rotation.
	lockAngularZ uint32 = 0x00000004

	// isFast is used for debug draw.
	isFast uint32 = 0x00000008

	// isBullet means this dynamic body does a final CCD pass against all body
	// types, but not other bullets.
	isBullet uint32 = 0x00000010

	// isSpeedCapped means this body was speed capped in the current time step.
	isSpeedCapped uint32 = 0x00000020

	// hadTimeOfImpact means this body had a time of impact event in the
	// current time step.
	hadTimeOfImpact uint32 = 0x00000040

	// allowFastRotation means this body has no limit on angular velocity.
	allowFastRotation uint32 = 0x00000080

	// enlargeBounds means this body needs to have its AABB increased.
	enlargeBounds uint32 = 0x00000100

	// dynamicFlag means this body is dynamic so the solver should write to
	// it. This prevents writing to kinematic bodies that causes a
	// multithreaded sharing cache coherence problem even when the values are
	// not changing. Used for bodyState flags.
	dynamicFlag uint32 = 0x00000200

	// dirtyMass indicates the user has used the updateBodyMass option to
	// defer mass computation but ApplyBodyMassFromShapes was not called
	// before the world step.
	dirtyMass uint32 = 0x00000400

	// allLocks is all lock flags.
	allLocks = lockAngularZ | lockLinearX | lockLinearY
)

// body holds organizational details that are not used in the solver
// (upstream b2Body).
type body struct {
	// Body debug name. Deviation from upstream: string instead of char[32].
	name string

	// User data. Deviation from upstream: the C void* becomes a uint64 so the
	// ECS wrapper can pack an entity id directly.
	userData uint64

	// index of solver set stored in World; may be NullIndex
	setIndex int

	// body sim and state index within set; may be NullIndex
	localIndex int

	// [31 : contactId | 1 : edgeIndex]
	headContactKey int
	contactCount   int

	// todo maybe move this to the body sim
	headShapeID int
	shapeCount  int

	headChainID int

	// [31 : jointId | 1 : edgeIndex]
	headJointKey int
	jointCount   int

	// All enabled dynamic and kinematic bodies are in an island.
	islandID int

	// Need this island index for faster union-find
	islandIndex int

	mass float64

	// Rotational inertia about the center of mass.
	inertia float64

	sleepThreshold float64
	sleepTime      float64

	// this is used to adjust the fellAsleep flag in the body move array
	bodyMoveIndex int

	id int

	// body flags (lockLinearX, ...)
	flags uint32

	bodyType BodyType

	// This is monotonically advanced when a body is allocated in this slot.
	// Used to check for invalid BodyID.
	generation uint16

	// todo move into flags
	enableSleep bool
}

// bodyState is the body state designed for fast conversion to and from SIMD
// via scatter-gather (upstream b2BodyState, 32 bytes). Only awake dynamic and
// kinematic bodies have a body state. This is used in the performance
// critical constraint solver. Keep the field order.
type bodyState struct {
	linearVelocity  Vec2
	angularVelocity float64

	// body flags; important flags: locking, dynamic
	flags uint32

	// Using delta position reduces round-off error far from the origin
	deltaPosition Vec2

	// Using delta rotation because the solver cannot access the full rotation
	// on static bodies and must use zero delta rotation for static bodies
	// (c,s) = (1,0)
	deltaRotation Rot
}

// identityBodyState is the identity body state, notice the deltaRotation is
// {1, 0} (upstream b2_identityBodyState).
var identityBodyState = bodyState{
	linearVelocity:  Vec2{X: 0.0, Y: 0.0},
	angularVelocity: 0.0,
	flags:           0,
	deltaPosition:   Vec2{X: 0.0, Y: 0.0},
	deltaRotation:   Rot{C: 1.0, S: 0.0},
}

// bodySim is body simulation data used for integration of position and
// velocity. Transform data used for collision and solver preparation
// (upstream b2BodySim).
type bodySim struct {
	// transform for body origin
	transform Transform

	// center of mass position in world space
	center Vec2

	// previous rotation and COM for TOI
	rotation0 Rot
	center0   Vec2

	// location of center of mass relative to the body origin
	localCenter Vec2

	force  Vec2
	torque float64

	// inverse inertia
	invMass    float64
	invInertia float64

	minExtent      float64
	maxExtent      float64
	linearDamping  float64
	angularDamping float64
	gravityScale   float64

	// Index of body
	bodyID int

	// body flags (lockLinearX, ...)
	flags uint32
}

// makeSweep builds a sweep from a body sim (upstream b2MakeSweep).
func makeSweep(sim *bodySim) Sweep {
	return Sweep{
		C1:          sim.center0,
		C2:          sim.center,
		Q1:          sim.rotation0,
		Q2:          sim.transform.Q,
		LocalCenter: sim.localCenter,
	}
}

// truncateName clamps a body name to nameLength-1 bytes, mirroring the
// upstream copy loop into char name[B2_NAME_LENGTH].
func truncateName(name string) string {
	if len(name) > nameLength-1 {
		return name[:nameLength-1]
	}
	return name
}

// limitVelocity clamps the linear velocity of a body state (upstream static
// b2LimitVelocity).
func limitVelocity(state *bodyState, maxLinearSpeed float64) {
	v2 := LengthSquared(state.linearVelocity)
	if v2 > float64(maxLinearSpeed*maxLinearSpeed) {
		state.linearVelocity = MulSV(maxLinearSpeed/math.Sqrt(v2), state.linearVelocity)
	}
}

// removeBodySim removes a body sim by swapping in the last element and fixing
// the moved body's localIndex (upstream b2RemoveBodySim).
func removeBodySim(bodySims *[]bodySim, bodies []body, localIndex int) {
	sims := *bodySims
	assert(0 <= localIndex && localIndex < len(sims))
	lastIndex := len(sims) - 1
	sims[localIndex] = sims[lastIndex]
	movedBody := &bodies[sims[localIndex].bodyID]
	assert(movedBody.localIndex == lastIndex)
	movedBody.localIndex = localIndex
	*bodySims = sims[:lastIndex]
}

// getBodyFullID returns a validated body from a world using an id
// (upstream b2GetBodyFullId).
func (w *World) getBodyFullID(bodyID BodyID) *body {
	assert(w.IsBodyValid(bodyID))

	// id index starts at one so that zero can represent null
	return &w.bodies[bodyID.index1-1]
}

// getBodyTransformQuick returns the transform of a body that is already
// looked up (upstream b2GetBodyTransformQuick).
func (w *World) getBodyTransformQuick(b *body) Transform {
	set := &w.solverSets[b.setIndex]
	return set.bodySims[b.localIndex].transform
}

// getBodyTransform returns the transform of a body by raw id
// (upstream b2GetBodyTransform).
func (w *World) getBodyTransform(bodyID int) Transform {
	b := &w.bodies[bodyID]
	return w.getBodyTransformQuick(b)
}

// makeBodyID creates a BodyID from a raw id (upstream b2MakeBodyId).
func (w *World) makeBodyID(bodyID int) BodyID {
	b := &w.bodies[bodyID]
	return BodyID{index1: int32(bodyID + 1), world0: w.worldID, generation: b.generation}
}

// getBodySim returns the body sim for a body (upstream b2GetBodySim).
func (w *World) getBodySim(b *body) *bodySim {
	set := &w.solverSets[b.setIndex]
	return &set.bodySims[b.localIndex]
}

// getBodyState returns the body state for a body, or nil if the body is not
// awake (upstream b2GetBodyState).
func (w *World) getBodyState(b *body) *bodyState {
	if b.setIndex == awakeSet {
		set := &w.solverSets[awakeSet]
		return &set.bodyStates[b.localIndex]
	}

	return nil
}

// createIslandForBody creates a single-body island (upstream static
// b2CreateIslandForBody).
func (w *World) createIslandForBody(setIndex int, b *body) {
	assert(b.islandID == NullIndex)
	assert(setIndex != disabledSet)

	isl := w.createIsland(setIndex)
	isl.bodies = append(isl.bodies, b.id)
	b.islandID = isl.islandID
	b.islandIndex = 0

	w.validateIsland(isl.islandID)
}

// removeBodyFromIsland removes a body from its island, destroying the island
// if it becomes empty (upstream static b2RemoveBodyFromIsland).
func (w *World) removeBodyFromIsland(b *body) {
	if b.islandID == NullIndex {
		assert(b.islandIndex == NullIndex)
		return
	}

	islandID := b.islandID
	isl := &w.islands[islandID]
	{
		localIndex := b.islandIndex
		movedBodyID := isl.bodies[len(isl.bodies)-1]
		isl.bodies[localIndex] = movedBodyID
		assert(w.bodies[movedBodyID].islandIndex == len(isl.bodies)-1)
		w.bodies[movedBodyID].islandIndex = localIndex
		isl.bodies = isl.bodies[:len(isl.bodies)-1]
	}

	if len(isl.bodies) == 0 {
		// Destroy empty island
		assert(len(isl.contacts) == 0)
		assert(len(isl.joints) == 0)

		// Free the island
		w.destroyIsland(isl.islandID)
	} else {
		w.validateIsland(islandID)
	}

	b.islandID = NullIndex
	b.islandIndex = NullIndex
}

// destroyBodyContacts destroys all contacts attached to a body (upstream
// static b2DestroyBodyContacts).
//

func (w *World) destroyBodyContacts(b *body, wakeBodies bool) {
	// Destroy the attached contacts
	edgeKey := b.headContactKey
	for edgeKey != NullIndex {
		contactID := edgeKey >> 1
		edgeIndex := edgeKey & 1

		c := &w.contacts[contactID]
		edgeKey = c.edges[edgeIndex].nextKey
		w.destroyContact(c, wakeBodies)
	}

	w.validateSolverSets()
}

// CreateBody creates a rigid body given a definition (upstream b2CreateBody).
// No reference to the definition is retained. So you can create the
// definition on the stack and pass it as a pointer.
//
// Warning: this function is locked during callbacks.
func (w *World) CreateBody(def *BodyDef) BodyID {
	assert(def.initialized)
	assert(IsValidVec2(def.Position))
	assert(IsValidRotation(def.Rotation))
	assert(IsValidVec2(def.LinearVelocity))
	assert(IsValidFloat(def.AngularVelocity))
	assert(IsValidFloat(def.LinearDamping) && def.LinearDamping >= 0.0)
	assert(IsValidFloat(def.AngularDamping) && def.AngularDamping >= 0.0)
	assert(IsValidFloat(def.SleepThreshold) && def.SleepThreshold >= 0.0)
	assert(IsValidFloat(def.GravityScale))

	assert(!w.locked)
	if w.locked {
		return BodyID{}
	}

	isAwake := (def.IsAwake || !def.EnableSleep) && def.IsEnabled

	// determine the solver set
	var setID int
	switch {
	case !def.IsEnabled:
		// any body type can be disabled
		setID = disabledSet
	case def.Type == StaticBody:
		setID = staticSet
	case isAwake:
		setID = awakeSet
	default:
		// new set for a sleeping body in its own island
		setID = allocID(&w.solverSetIDPool)
		if setID == len(w.solverSets) {
			// Create a zero initialized solver set. All sub-arrays are also
			// zero initialized.
			w.solverSets = append(w.solverSets, solverSet{})
		} else {
			assert(w.solverSets[setID].setIndex == NullIndex)
		}

		w.solverSets[setID].setIndex = setID
	}

	assert(0 <= setID && setID < len(w.solverSets))

	bodyID := allocID(&w.bodyIDPool)

	var lockFlags uint32
	if def.MotionLocks.LinearX {
		lockFlags |= lockLinearX
	}
	if def.MotionLocks.LinearY {
		lockFlags |= lockLinearY
	}
	if def.MotionLocks.AngularZ {
		lockFlags |= lockAngularZ
	}

	set := &w.solverSets[setID]
	set.bodySims = append(set.bodySims, bodySim{})
	bSim := &set.bodySims[len(set.bodySims)-1]
	bSim.transform.P = def.Position
	bSim.transform.Q = def.Rotation
	bSim.center = def.Position
	bSim.rotation0 = bSim.transform.Q
	bSim.center0 = bSim.center
	bSim.minExtent = Huge
	bSim.maxExtent = 0.0
	bSim.linearDamping = def.LinearDamping
	bSim.angularDamping = def.AngularDamping
	bSim.gravityScale = def.GravityScale
	bSim.bodyID = bodyID
	bSim.flags = lockFlags
	if def.IsBullet {
		bSim.flags |= isBullet
	}
	if def.AllowFastRotation {
		bSim.flags |= allowFastRotation
	}
	if def.Type == DynamicBody {
		bSim.flags |= dynamicFlag
	}

	if setID == awakeSet {
		state := bodyState{}
		state.linearVelocity = def.LinearVelocity
		state.angularVelocity = def.AngularVelocity
		state.deltaRotation = RotIdentity
		state.flags = bSim.flags
		set.bodyStates = append(set.bodyStates, state)
	}

	if bodyID == len(w.bodies) {
		w.bodies = append(w.bodies, body{})
	} else {
		assert(w.bodies[bodyID].id == NullIndex)
	}

	b := &w.bodies[bodyID]

	b.name = truncateName(def.Name)
	b.userData = def.UserData
	b.setIndex = setID
	b.localIndex = len(set.bodySims) - 1
	b.generation++
	b.headShapeID = NullIndex
	b.shapeCount = 0
	b.headChainID = NullIndex
	b.headContactKey = NullIndex
	b.contactCount = 0
	b.headJointKey = NullIndex
	b.jointCount = 0
	b.islandID = NullIndex
	b.islandIndex = NullIndex
	b.bodyMoveIndex = NullIndex
	b.id = bodyID
	b.mass = 0.0
	b.inertia = 0.0
	b.sleepThreshold = def.SleepThreshold
	b.sleepTime = 0.0
	b.bodyType = def.Type
	b.flags = bSim.flags
	b.enableSleep = def.EnableSleep

	// dynamic and kinematic bodies that are enabled need a island
	if setID >= awakeSet {
		w.createIslandForBody(setID, b)
	}

	w.validateSolverSets()

	return BodyID{index1: int32(bodyID + 1), world0: w.worldID, generation: b.generation}
}

// wakeBody wakes a body's solver set if it is sleeping and reports whether
// anything was woken (upstream b2WakeBody).
//
// Careful calling this because it can invalidate body, state, joint, and
// contact pointers.
//
//nolint:unparam // the bool result mirrors the upstream signature; E6+ call sites use it.
func (w *World) wakeBody(b *body) bool {
	if b.setIndex >= firstSleepingSet {
		w.wakeSolverSet(b.setIndex)
		w.validateSolverSets()
		return true
	}

	return false
}

// DestroyBody destroys a rigid body given an id. This destroys all shapes and
// joints attached to the body. Do not keep references to the associated
// shapes and joints (upstream b2DestroyBody).
func (w *World) DestroyBody(bodyID BodyID) {
	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)

	// Wake bodies attached to this body, even if this body is static.
	wakeBodies := true

	// Destroy the attached joints
	edgeKey := b.headJointKey
	for edgeKey != NullIndex {
		jointID := edgeKey >> 1
		edgeIndex := edgeKey & 1

		j := &w.joints[jointID]
		edgeKey = j.edges[edgeIndex].nextKey

		// Careful because this modifies the list being traversed
		w.destroyJointInternal(j, wakeBodies)
	}

	// Destroy all contacts attached to this body.
	w.destroyBodyContacts(b, wakeBodies)

	// Destroy the attached shapes and their broad-phase proxies.
	shapeID := b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]

		if s.sensorIndex != NullIndex {
			w.destroySensor(s)
		}

		destroyShapeProxy(s, &w.broadPhase)

		// Return shape to free list.
		freeID(&w.shapeIDPool, shapeID)
		s.id = NullIndex

		shapeID = s.nextShapeID
	}

	// Destroy the attached chains. The associated shapes have already been
	// destroyed above.
	chainID := b.headChainID
	for chainID != NullIndex {
		chain := &w.chainShapes[chainID]

		freeChainData(chain)

		// Return chain to free list.
		freeID(&w.chainIDPool, chainID)
		chain.id = NullIndex

		chainID = chain.nextChainID
	}

	w.removeBodyFromIsland(b)

	// Remove body sim from solver set that owns it
	set := &w.solverSets[b.setIndex]
	removeBodySim(&set.bodySims, w.bodies, b.localIndex)

	// Remove body state from awake set
	if b.setIndex == awakeSet {
		_ = removeSwap(&set.bodyStates, b.localIndex)
	} else if set.setIndex >= firstSleepingSet && len(set.bodySims) == 0 {
		// Remove solver set if it is empty
		w.destroySolverSet(set.setIndex)
	}

	// Free body and id (preserve body generation)
	freeID(&w.bodyIDPool, b.id)

	b.setIndex = NullIndex
	b.localIndex = NullIndex
	b.id = NullIndex

	w.validateSolverSets()
}

// BodyContactCapacity returns the maximum capacity required for retrieving
// all the touching contacts on a body (upstream b2Body_GetContactCapacity).
func (w *World) BodyContactCapacity(bodyID BodyID) int {
	if w.locked {
		assert(false)
		return 0
	}

	b := w.getBodyFullID(bodyID)

	// Conservative and fast
	return b.contactCount
}

// BodyContactData fills contactData with the touching contact data for a
// body, up to len(contactData) elements, and returns the number of elements
// stored (upstream b2Body_GetContactData).
func (w *World) BodyContactData(bodyID BodyID, contactData []ContactData) int {
	if w.locked {
		assert(false)
		return 0
	}

	b := w.getBodyFullID(bodyID)

	contactKey := b.headContactKey
	index := 0
	for contactKey != NullIndex && index < len(contactData) {
		contactID := contactKey >> 1
		edgeIndex := contactKey & 1

		c := &w.contacts[contactID]

		// Is contact touching?
		if c.flags&contactTouchingFlag != 0 {
			shapeA := &w.shapes[c.shapeIDA]
			shapeB := &w.shapes[c.shapeIDB]

			contactData[index].ContactID = ContactID{
				index1:     int32(c.contactID + 1),
				world0:     bodyID.world0,
				padding:    0,
				generation: c.generation,
			}
			contactData[index].ShapeIDA = ShapeID{
				index1:     int32(shapeA.id + 1),
				world0:     bodyID.world0,
				generation: shapeA.generation,
			}
			contactData[index].ShapeIDB = ShapeID{
				index1:     int32(shapeB.id + 1),
				world0:     bodyID.world0,
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

// ComputeBodyAABB returns the current world AABB that contains all the
// attached shapes. Note that this may not encompass the body origin. If there
// are no shapes attached then the returned AABB is empty and centered on the
// body origin (upstream b2Body_ComputeAABB).
func (w *World) ComputeBodyAABB(bodyID BodyID) AABB {
	if w.locked {
		assert(false)
		return AABB{}
	}

	b := w.getBodyFullID(bodyID)
	if b.headShapeID == NullIndex {
		transform := w.getBodyTransform(b.id)
		return AABB{LowerBound: transform.P, UpperBound: transform.P}
	}

	s := &w.shapes[b.headShapeID]
	aabb := s.aabb
	for s.nextShapeID != NullIndex {
		s = &w.shapes[s.nextShapeID]
		aabb = AABBUnion(aabb, s.aabb)
	}

	return aabb
}

// updateBodyMassData recomputes a body's mass, center of mass and rotational
// inertia from its shapes (upstream b2UpdateBodyMassData).
func (w *World) updateBodyMassData(b *body) {
	bSim := w.getBodySim(b)

	// Mass is no longer dirty
	b.flags &^= dirtyMass

	// Compute mass data from shapes. Each shape has its own density.
	b.mass = 0.0
	b.inertia = 0.0

	bSim.invMass = 0.0
	bSim.invInertia = 0.0
	bSim.localCenter = Vec2Zero
	bSim.minExtent = Huge
	bSim.maxExtent = 0.0

	// Static and kinematic sims have zero mass.
	if b.bodyType != DynamicBody {
		bSim.center = bSim.transform.P
		bSim.center0 = bSim.center

		// Need extents for kinematic bodies for sleeping to work correctly.
		if b.bodyType == KinematicBody {
			shapeID := b.headShapeID
			for shapeID != NullIndex {
				s := &w.shapes[shapeID]

				extent := computeShapeExtent(s, Vec2Zero)
				bSim.minExtent = minFloat(bSim.minExtent, extent.minExtent)
				bSim.maxExtent = maxFloat(bSim.maxExtent, extent.maxExtent)

				shapeID = s.nextShapeID
			}
		}

		return
	}

	shapeCount := b.shapeCount
	masses := w.arena.allocMassData(shapeCount)

	// Accumulate mass over all shapes.
	localCenter := Vec2Zero
	shapeID := b.headShapeID
	shapeIndex := 0
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]
		shapeID = s.nextShapeID

		if s.density == 0.0 {
			masses[shapeIndex] = MassData{}
			shapeIndex++
			continue
		}

		massData := computeShapeMass(s)
		b.mass += massData.Mass
		localCenter = MulAdd(localCenter, massData.Mass, massData.Center)

		masses[shapeIndex] = massData
		shapeIndex++
	}

	// Compute center of mass.
	if b.mass > 0.0 {
		bSim.invMass = 1.0 / b.mass
		localCenter = MulSV(bSim.invMass, localCenter)
	}

	// Second loop to accumulate the rotational inertia about the center of mass
	for shapeIndex = range shapeCount {
		massData := masses[shapeIndex]
		if massData.Mass == 0.0 {
			continue
		}

		// Shift to center of mass. This is safe because it can only increase.
		offset := Sub(localCenter, massData.Center)
		inertia := mulAdd(massData.Mass, Dot(offset, offset), massData.RotationalInertia)
		b.inertia += inertia
	}

	w.arena.freeMassData()

	assert(b.inertia >= 0.0)

	if b.inertia > 0.0 {
		bSim.invInertia = 1.0 / b.inertia
	} else {
		b.inertia = 0.0
		bSim.invInertia = 0.0
	}

	// Move center of mass.
	oldCenter := bSim.center
	bSim.localCenter = localCenter
	bSim.center = TransformPoint(bSim.transform, bSim.localCenter)
	bSim.center0 = bSim.center

	// Update center of mass velocity
	state := w.getBodyState(b)
	if state != nil {
		deltaLinear := CrossSV(state.angularVelocity, Sub(bSim.center, oldCenter))
		state.linearVelocity = Add(state.linearVelocity, deltaLinear)
	}

	// Compute body extents relative to center of mass
	shapeID = b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]

		extent := computeShapeExtent(s, localCenter)
		bSim.minExtent = minFloat(bSim.minExtent, extent.minExtent)
		bSim.maxExtent = maxFloat(bSim.maxExtent, extent.maxExtent)

		shapeID = s.nextShapeID
	}
}

// BodyPosition returns the world position of a body. This is the location of
// the body origin (upstream b2Body_GetPosition).
func (w *World) BodyPosition(bodyID BodyID) Vec2 {
	b := w.getBodyFullID(bodyID)
	return w.getBodyTransformQuick(b).P
}

// BodyRotation returns the world rotation of a body as a cosine/sine pair
// (upstream b2Body_GetRotation).
func (w *World) BodyRotation(bodyID BodyID) Rot {
	b := w.getBodyFullID(bodyID)
	return w.getBodyTransformQuick(b).Q
}

// BodyTransform returns the world transform of a body
// (upstream b2Body_GetTransform).
func (w *World) BodyTransform(bodyID BodyID) Transform {
	b := w.getBodyFullID(bodyID)
	return w.getBodyTransformQuick(b)
}

// BodyLocalPoint returns a local point on a body given a world point
// (upstream b2Body_GetLocalPoint).
func (w *World) BodyLocalPoint(bodyID BodyID, worldPoint Vec2) Vec2 {
	b := w.getBodyFullID(bodyID)
	transform := w.getBodyTransformQuick(b)
	return InvTransformPoint(transform, worldPoint)
}

// BodyWorldPoint returns a world point on a body given a local point
// (upstream b2Body_GetWorldPoint).
func (w *World) BodyWorldPoint(bodyID BodyID, localPoint Vec2) Vec2 {
	b := w.getBodyFullID(bodyID)
	transform := w.getBodyTransformQuick(b)
	return TransformPoint(transform, localPoint)
}

// BodyLocalVector returns a local vector on a body given a world vector
// (upstream b2Body_GetLocalVector).
func (w *World) BodyLocalVector(bodyID BodyID, worldVector Vec2) Vec2 {
	b := w.getBodyFullID(bodyID)
	transform := w.getBodyTransformQuick(b)
	return InvRotateVector(transform.Q, worldVector)
}

// BodyWorldVector returns a world vector on a body given a local vector
// (upstream b2Body_GetWorldVector).
func (w *World) BodyWorldVector(bodyID BodyID, localVector Vec2) Vec2 {
	b := w.getBodyFullID(bodyID)
	transform := w.getBodyTransformQuick(b)
	return RotateVector(transform.Q, localVector)
}

// SetBodyTransform sets the world transform of a body. This acts as a
// teleport and is fairly expensive.
//
// Note: generally you should create a body with the intended transform.
// (upstream b2Body_SetTransform).
func (w *World) SetBodyTransform(bodyID BodyID, position Vec2, rotation Rot) {
	assert(IsValidVec2(position))
	assert(IsValidRotation(rotation))
	assert(w.IsBodyValid(bodyID))
	assert(!w.locked)

	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)

	bSim.transform.P = position
	bSim.transform.Q = rotation
	bSim.center = TransformPoint(bSim.transform, bSim.localCenter)

	bSim.rotation0 = bSim.transform.Q
	bSim.center0 = bSim.center

	bp := &w.broadPhase

	transform := bSim.transform
	const speculativeDistance = SpeculativeDistance

	shapeID := b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]
		aabb := computeShapeAABB(s, transform)
		aabb.LowerBound.X -= speculativeDistance
		aabb.LowerBound.Y -= speculativeDistance
		aabb.UpperBound.X += speculativeDistance
		aabb.UpperBound.Y += speculativeDistance
		s.aabb = aabb

		if !AABBContains(s.fatAABB, aabb) {
			margin := s.aabbMargin
			var fatAABB AABB
			fatAABB.LowerBound.X = aabb.LowerBound.X - margin
			fatAABB.LowerBound.Y = aabb.LowerBound.Y - margin
			fatAABB.UpperBound.X = aabb.UpperBound.X + margin
			fatAABB.UpperBound.Y = aabb.UpperBound.Y + margin
			s.fatAABB = fatAABB

			// The body could be disabled
			if s.proxyKey != NullIndex {
				bp.moveProxy(s.proxyKey, fatAABB)
			}
		}

		shapeID = s.nextShapeID
	}
}

// BodyLinearVelocity returns the linear velocity of a body's center of mass,
// usually in meters per second (upstream b2Body_GetLinearVelocity).
func (w *World) BodyLinearVelocity(bodyID BodyID) Vec2 {
	b := w.getBodyFullID(bodyID)
	state := w.getBodyState(b)
	if state != nil {
		return state.linearVelocity
	}
	return Vec2Zero
}

// BodyAngularVelocity returns the angular velocity of a body in radians per
// second (upstream b2Body_GetAngularVelocity).
func (w *World) BodyAngularVelocity(bodyID BodyID) float64 {
	b := w.getBodyFullID(bodyID)
	state := w.getBodyState(b)
	if state != nil {
		return state.angularVelocity
	}
	return 0.0
}

// SetBodyLinearVelocity sets the linear velocity of a body, usually in meters
// per second (upstream b2Body_SetLinearVelocity).
func (w *World) SetBodyLinearVelocity(bodyID BodyID, linearVelocity Vec2) {
	b := w.getBodyFullID(bodyID)

	if b.bodyType == StaticBody {
		return
	}

	if LengthSquared(linearVelocity) > 0.0 {
		w.wakeBody(b)
	}

	state := w.getBodyState(b)
	if state == nil {
		return
	}

	state.linearVelocity = linearVelocity
}

// SetBodyAngularVelocity sets the angular velocity of a body in radians per
// second (upstream b2Body_SetAngularVelocity).
func (w *World) SetBodyAngularVelocity(bodyID BodyID, angularVelocity float64) {
	b := w.getBodyFullID(bodyID)

	if b.bodyType == StaticBody || b.flags&lockAngularZ != 0 {
		return
	}

	if angularVelocity != 0.0 {
		w.wakeBody(b)
	}

	state := w.getBodyState(b)
	if state == nil {
		return
	}

	state.angularVelocity = angularVelocity
}

// SetBodyTargetTransform sets the velocity to reach the given transform after
// a given time step. The result will be close but maybe not exact. This makes
// it difficult to push a body that is connected with joints or in contact
// with heavy bodies (upstream b2Body_SetTargetTransform).
func (w *World) SetBodyTargetTransform(bodyID BodyID, target Transform, timeStep float64, wake bool) {
	b := w.getBodyFullID(bodyID)

	if b.setIndex == disabledSet {
		return
	}

	if b.bodyType == StaticBody || timeStep <= 0.0 {
		return
	}

	if b.setIndex != awakeSet && !wake {
		return
	}

	sim := w.getBodySim(b)

	// Compute linear velocity
	center1 := sim.center
	center2 := TransformPoint(target, sim.localCenter)
	invTimeStep := 1.0 / timeStep
	linearVelocity := MulSV(invTimeStep, Sub(center2, center1))

	// Compute angular velocity
	q1 := sim.transform.Q
	q2 := target.Q
	deltaAngle := RelativeAngle(q1, q2)
	angularVelocity := invTimeStep * deltaAngle

	// Early out if the body is asleep already and the desired movement is small
	if b.setIndex != awakeSet {
		maxVelocity := mulAdd(absFloat(angularVelocity), sim.maxExtent, Length(linearVelocity))

		// Return if velocity would be sleepy
		if maxVelocity < b.sleepThreshold {
			return
		}

		// Must wake for state to exist
		w.wakeBody(b)
	}

	assert(b.setIndex == awakeSet)

	state := w.getBodyState(b)
	state.linearVelocity = linearVelocity
	state.angularVelocity = angularVelocity
}

// BodyLocalPointVelocity returns the linear velocity of a local point
// attached to a body, usually in meters per second
// (upstream b2Body_GetLocalPointVelocity).
func (w *World) BodyLocalPointVelocity(bodyID BodyID, localPoint Vec2) Vec2 {
	b := w.getBodyFullID(bodyID)
	state := w.getBodyState(b)
	if state == nil {
		return Vec2Zero
	}

	set := &w.solverSets[b.setIndex]
	bSim := &set.bodySims[b.localIndex]

	r := RotateVector(bSim.transform.Q, Sub(localPoint, bSim.localCenter))
	return Add(state.linearVelocity, CrossSV(state.angularVelocity, r))
}

// BodyWorldPointVelocity returns the linear velocity of a world point
// attached to a body, usually in meters per second
// (upstream b2Body_GetWorldPointVelocity).
func (w *World) BodyWorldPointVelocity(bodyID BodyID, worldPoint Vec2) Vec2 {
	b := w.getBodyFullID(bodyID)
	state := w.getBodyState(b)
	if state == nil {
		return Vec2Zero
	}

	set := &w.solverSets[b.setIndex]
	bSim := &set.bodySims[b.localIndex]

	r := Sub(worldPoint, bSim.center)
	return Add(state.linearVelocity, CrossSV(state.angularVelocity, r))
}

// ApplyBodyForce applies a force at a world point. If the force is not
// applied at the center of mass, it will generate a torque and affect the
// angular velocity. The force is ignored if the body is not awake
// (upstream b2Body_ApplyForce).
func (w *World) ApplyBodyForce(bodyID BodyID, force, point Vec2, wake bool) {
	b := w.getBodyFullID(bodyID)

	if b.bodyType != DynamicBody || b.setIndex == disabledSet {
		return
	}

	if wake && b.setIndex >= firstSleepingSet {
		w.wakeBody(b)
	}

	if b.setIndex == awakeSet {
		bSim := w.getBodySim(b)
		bSim.force = Add(bSim.force, force)
		bSim.torque += Cross(Sub(point, bSim.center), force)
	}
}

// ApplyBodyForceToCenter applies a force to the center of mass. The force is
// ignored if the body is not awake (upstream b2Body_ApplyForceToCenter).
func (w *World) ApplyBodyForceToCenter(bodyID BodyID, force Vec2, wake bool) {
	b := w.getBodyFullID(bodyID)

	if b.bodyType != DynamicBody || b.setIndex == disabledSet {
		return
	}

	if wake && b.setIndex >= firstSleepingSet {
		w.wakeBody(b)
	}

	if b.setIndex == awakeSet {
		bSim := w.getBodySim(b)
		bSim.force = Add(bSim.force, force)
	}
}

// ApplyBodyTorque applies a torque. This affects the angular velocity without
// affecting the linear velocity. The torque is ignored if the body is not
// awake (upstream b2Body_ApplyTorque).
func (w *World) ApplyBodyTorque(bodyID BodyID, torque float64, wake bool) {
	b := w.getBodyFullID(bodyID)

	if b.bodyType != DynamicBody || b.setIndex == disabledSet {
		return
	}

	if wake && b.setIndex >= firstSleepingSet {
		w.wakeBody(b)
	}

	if b.setIndex == awakeSet {
		bSim := w.getBodySim(b)
		bSim.torque += torque
	}
}

// ClearBodyForces clears the accumulated force and torque on a body
// (upstream b2Body_ClearForces).
func (w *World) ClearBodyForces(bodyID BodyID) {
	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	bSim.force = Vec2Zero
	bSim.torque = 0.0
}

// ApplyBodyLinearImpulse applies an impulse at a point. This immediately
// modifies the velocity. It also modifies the angular velocity if the point
// of application is not at the center of mass. The impulse is ignored if the
// body is not awake (upstream b2Body_ApplyLinearImpulse).
//
// Warning: this should be used for one-shot impulses. If you need a steady
// force, use a force instead, which will work better with the sub-stepping
// solver.
func (w *World) ApplyBodyLinearImpulse(bodyID BodyID, impulse, point Vec2, wake bool) {
	b := w.getBodyFullID(bodyID)

	if b.bodyType != DynamicBody || b.setIndex == disabledSet {
		return
	}

	if wake && b.setIndex >= firstSleepingSet {
		w.wakeBody(b)
	}

	if b.setIndex == awakeSet {
		localIndex := b.localIndex
		set := &w.solverSets[awakeSet]
		state := &set.bodyStates[localIndex]
		bSim := &set.bodySims[localIndex]
		state.linearVelocity = MulAdd(state.linearVelocity, bSim.invMass, impulse)
		state.angularVelocity = mulAdd(bSim.invInertia, Cross(Sub(point, bSim.center), impulse), state.angularVelocity)

		limitVelocity(state, w.maxLinearSpeed)
	}
}

// ApplyBodyLinearImpulseToCenter applies an impulse to the center of mass.
// This immediately modifies the velocity. The impulse is ignored if the body
// is not awake (upstream b2Body_ApplyLinearImpulseToCenter).
//
// Warning: this should be used for one-shot impulses. If you need a steady
// force, use a force instead, which will work better with the sub-stepping
// solver.
func (w *World) ApplyBodyLinearImpulseToCenter(bodyID BodyID, impulse Vec2, wake bool) {
	b := w.getBodyFullID(bodyID)

	if b.bodyType != DynamicBody || b.setIndex == disabledSet {
		return
	}

	if wake && b.setIndex >= firstSleepingSet {
		w.wakeBody(b)
	}

	if b.setIndex == awakeSet {
		localIndex := b.localIndex
		set := &w.solverSets[awakeSet]
		state := &set.bodyStates[localIndex]
		bSim := &set.bodySims[localIndex]
		state.linearVelocity = MulAdd(state.linearVelocity, bSim.invMass, impulse)

		limitVelocity(state, w.maxLinearSpeed)
	}
}

// ApplyBodyAngularImpulse applies an angular impulse. The impulse is ignored
// if the body is not awake (upstream b2Body_ApplyAngularImpulse).
//
// Warning: this should be used for one-shot impulses. If you need a steady
// torque, use a torque instead, which will work better with the sub-stepping
// solver.
func (w *World) ApplyBodyAngularImpulse(bodyID BodyID, impulse float64, wake bool) {
	assert(w.IsBodyValid(bodyID))
	b := w.getBodyFullID(bodyID)

	if b.bodyType != DynamicBody || b.setIndex == disabledSet {
		return
	}

	if wake && b.setIndex >= firstSleepingSet {
		// this will not invalidate body pointer
		w.wakeBody(b)
	}

	if b.setIndex == awakeSet {
		localIndex := b.localIndex
		set := &w.solverSets[awakeSet]
		state := &set.bodyStates[localIndex]
		bSim := &set.bodySims[localIndex]
		state.angularVelocity = mulAdd(bSim.invInertia, impulse, state.angularVelocity)
	}
}

// BodyType returns the body type: static, kinematic, or dynamic
// (upstream b2Body_GetType).
func (w *World) BodyType(bodyID BodyID) BodyType {
	b := w.getBodyFullID(bodyID)
	return b.bodyType
}

// SetBodyType changes the body type. This is an expensive operation. This
// automatically updates the mass properties regardless of the automatic mass
// setting (upstream b2Body_SetType).
//
// This should follow similar steps as you would get destroying and recreating
// the body, shapes, and joints. Contacts are difficult to preserve because
// the broad-phase pairs change, so upstream just destroys them.
//
// Revised steps (upstream comment):
//  1. Skip disabled bodies
//  2. Destroy all contacts on the body
//  3. Wake the body
//  4. For all joints attached to the body
//     - wake attached bodies
//     - remove from island
//     - move to static set temporarily
//  5. Change the body type and transfer the body
//  6. If the body was static
//     - create an island for the body
//     Else if the body is becoming static
//     - remove it from the island
//  7. For all joints
//     - if either body is non-static
//     - link into island
//     - transfer to constraint graph
//  8. For all shapes
//     - Destroy proxy in old tree
//     - Create proxy in new tree
//
// Notes:
//   - the implementation below tries to minimize the number of predicates, so
//     some operations may have no effect, such as transferring a joint to the
//     same set
func (w *World) SetBodyType(bodyID BodyID, bodyType BodyType) {
	b := w.getBodyFullID(bodyID)

	originalType := b.bodyType
	if originalType == bodyType {
		return
	}

	// Stage 1: skip disabled bodies
	if b.setIndex == disabledSet {
		// Disabled bodies don't change solver sets or islands when they
		// change type.
		b.bodyType = bodyType

		if bodyType == DynamicBody {
			b.flags |= dynamicFlag
		} else {
			b.flags &^= dynamicFlag
		}

		// Body type affects the mass properties
		w.updateBodyMassData(b)
		return
	}

	// Stage 2: destroy all contacts but don't wake bodies (because we don't
	// need to)
	wakeBodies := false
	w.destroyBodyContacts(b, wakeBodies)

	// Stage 3: wake this body (does nothing if body is static), otherwise it
	// will also wake all bodies in the same sleeping solver set.
	w.wakeBody(b)

	// Stage 4: move joints to temporary storage
	staticSolverSet := &w.solverSets[staticSet]

	jointKey := b.headJointKey
	for jointKey != NullIndex {
		jointID := jointKey >> 1
		edgeIndex := jointKey & 1

		j := &w.joints[jointID]
		jointKey = j.edges[edgeIndex].nextKey

		// Joint may be disabled by other body
		if j.setIndex == disabledSet {
			continue
		}

		// Wake attached bodies. The wakeBody call above does not wake bodies
		// attached to a static body. But it is necessary because the body may
		// have no joints.
		bodyA := &w.bodies[j.edges[0].bodyID]
		bodyB := &w.bodies[j.edges[1].bodyID]
		w.wakeBody(bodyA)
		w.wakeBody(bodyB)

		// Remove joint from island
		w.unlinkJoint(j)

		// It is necessary to transfer all joints to the static set so they
		// can be added to the constraint graph below and acquire consistent
		// colors.
		jointSourceSet := &w.solverSets[j.setIndex]
		w.transferJoint(staticSolverSet, jointSourceSet, j)
	}

	// Stage 5: change the body type and transfer body
	b.bodyType = bodyType

	if bodyType == DynamicBody {
		b.flags |= dynamicFlag
	} else {
		b.flags &^= dynamicFlag
	}

	awakeSolverSet := &w.solverSets[awakeSet]
	sourceSet := &w.solverSets[b.setIndex]
	targetSet := awakeSolverSet
	if bodyType == StaticBody {
		targetSet = staticSolverSet
	}

	// Transfer body
	w.transferBody(targetSet, sourceSet, b)

	// Stage 6: update island participation for the body
	if originalType == StaticBody {
		// Create island for body
		w.createIslandForBody(awakeSet, b)
	} else if bodyType == StaticBody {
		// Remove body from island.
		w.removeBodyFromIsland(b)
	}

	// Stage 7: Transfer joints to the target set
	jointKey = b.headJointKey
	for jointKey != NullIndex {
		jointID := jointKey >> 1
		edgeIndex := jointKey & 1

		j := &w.joints[jointID]

		jointKey = j.edges[edgeIndex].nextKey

		// Joint may be disabled by other body
		if j.setIndex == disabledSet {
			continue
		}

		// All joints were transferred to the static set in an earlier stage
		assert(j.setIndex == staticSet)

		bodyA := &w.bodies[j.edges[0].bodyID]
		bodyB := &w.bodies[j.edges[1].bodyID]
		assert(bodyA.setIndex == staticSet || bodyA.setIndex == awakeSet)
		assert(bodyB.setIndex == staticSet || bodyB.setIndex == awakeSet)

		if bodyA.bodyType == DynamicBody || bodyB.bodyType == DynamicBody {
			w.transferJoint(awakeSolverSet, staticSolverSet, j)
		}
	}

	// Recreate shape proxies in broadphase
	transform := w.getBodyTransformQuick(b)
	shapeID := b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]
		shapeID = s.nextShapeID
		destroyShapeProxy(s, &w.broadPhase)
		forcePairCreation := true
		createShapeProxy(s, &w.broadPhase, bodyType, transform, forcePairCreation)
	}

	// Relink all joints
	jointKey = b.headJointKey
	for jointKey != NullIndex {
		jointID := jointKey >> 1
		edgeIndex := jointKey & 1

		j := &w.joints[jointID]
		jointKey = j.edges[edgeIndex].nextKey

		otherEdgeIndex := edgeIndex ^ 1
		otherBodyID := j.edges[otherEdgeIndex].bodyID
		otherBody := &w.bodies[otherBodyID]

		if otherBody.setIndex == disabledSet {
			continue
		}

		if b.bodyType != DynamicBody && otherBody.bodyType != DynamicBody {
			continue
		}

		w.linkJoint(j)
	}

	// Body type affects the mass
	w.updateBodyMassData(b)

	state := w.getBodyState(b)
	if state != nil {
		// Ensure flags are in sync (b2_skipSolverWrite)
		state.flags = b.flags
	}

	w.validateSolverSets()
	w.validateIsland(b.islandID)
}

// SetBodyName sets the body name, up to 31 bytes (upstream b2Body_SetName).
func (w *World) SetBodyName(bodyID BodyID, name string) {
	b := w.getBodyFullID(bodyID)
	b.name = truncateName(name)
}

// BodyName returns the body name (upstream b2Body_GetName).
func (w *World) BodyName(bodyID BodyID) string {
	b := w.getBodyFullID(bodyID)
	return b.name
}

// SetBodyUserData sets the user data on a body (upstream b2Body_SetUserData).
func (w *World) SetBodyUserData(bodyID BodyID, userData uint64) {
	b := w.getBodyFullID(bodyID)
	b.userData = userData
}

// BodyUserData returns the user data stored on a body
// (upstream b2Body_GetUserData).
func (w *World) BodyUserData(bodyID BodyID) uint64 {
	b := w.getBodyFullID(bodyID)
	return b.userData
}

// BodyMass returns the mass of the body, usually in kilograms
// (upstream b2Body_GetMass).
func (w *World) BodyMass(bodyID BodyID) float64 {
	b := w.getBodyFullID(bodyID)
	return b.mass
}

// BodyRotationalInertia returns the rotational inertia of the body, usually
// in kg*m^2 (upstream b2Body_GetRotationalInertia).
func (w *World) BodyRotationalInertia(bodyID BodyID) float64 {
	b := w.getBodyFullID(bodyID)
	return b.inertia
}

// BodyLocalCenterOfMass returns the center of mass position of the body in
// local space (upstream b2Body_GetLocalCenterOfMass).
func (w *World) BodyLocalCenterOfMass(bodyID BodyID) Vec2 {
	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	return bSim.localCenter
}

// BodyWorldCenterOfMass returns the center of mass position of the body in
// world space (upstream b2Body_GetWorldCenterOfMass).
func (w *World) BodyWorldCenterOfMass(bodyID BodyID) Vec2 {
	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	return bSim.center
}

// SetBodyMassData overrides the body's mass properties. Normally this is
// computed automatically using the shape geometry and density. This
// information is lost if a shape is added or removed or if the body type
// changes (upstream b2Body_SetMassData).
func (w *World) SetBodyMassData(bodyID BodyID, massData MassData) {
	assert(IsValidFloat(massData.Mass) && massData.Mass >= 0.0)
	assert(IsValidFloat(massData.RotationalInertia) && massData.RotationalInertia >= 0.0)
	assert(IsValidVec2(massData.Center))

	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)

	b.mass = massData.Mass
	b.inertia = massData.RotationalInertia
	bSim.localCenter = massData.Center

	center := TransformPoint(bSim.transform, massData.Center)
	bSim.center = center
	bSim.center0 = center

	if b.mass > 0.0 {
		bSim.invMass = 1.0 / b.mass
	} else {
		bSim.invMass = 0.0
	}

	if b.inertia > 0.0 {
		bSim.invInertia = 1.0 / b.inertia
	} else {
		bSim.invInertia = 0.0
	}
}

// BodyMassData returns the mass data for a body (upstream b2Body_GetMassData).
func (w *World) BodyMassData(bodyID BodyID) MassData {
	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	return MassData{Mass: b.mass, Center: bSim.localCenter, RotationalInertia: b.inertia}
}

// ApplyBodyMassFromShapes updates the body mass data. This should be called
// if you have added or removed shapes without automatic mass computation, or
// if you have directly modified a shape's density
// (upstream b2Body_ApplyMassFromShapes).
func (w *World) ApplyBodyMassFromShapes(bodyID BodyID) {
	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	w.updateBodyMassData(b)
}

// SetBodyLinearDamping adjusts the linear damping. Normally this is set in
// BodyDef before creation (upstream b2Body_SetLinearDamping).
func (w *World) SetBodyLinearDamping(bodyID BodyID, linearDamping float64) {
	assert(IsValidFloat(linearDamping) && linearDamping >= 0.0)

	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	bSim.linearDamping = linearDamping
}

// BodyLinearDamping returns the current linear damping
// (upstream b2Body_GetLinearDamping).
func (w *World) BodyLinearDamping(bodyID BodyID) float64 {
	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	return bSim.linearDamping
}

// SetBodyAngularDamping adjusts the angular damping. Normally this is set in
// BodyDef before creation (upstream b2Body_SetAngularDamping).
func (w *World) SetBodyAngularDamping(bodyID BodyID, angularDamping float64) {
	assert(IsValidFloat(angularDamping) && angularDamping >= 0.0)

	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	bSim.angularDamping = angularDamping
}

// BodyAngularDamping returns the current angular damping
// (upstream b2Body_GetAngularDamping).
func (w *World) BodyAngularDamping(bodyID BodyID) float64 {
	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	return bSim.angularDamping
}

// SetBodyGravityScale adjusts the gravity scale. Normally this is set in
// BodyDef before creation (upstream b2Body_SetGravityScale).
func (w *World) SetBodyGravityScale(bodyID BodyID, gravityScale float64) {
	assert(w.IsBodyValid(bodyID))
	assert(IsValidFloat(gravityScale))

	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	bSim.gravityScale = gravityScale
}

// BodyGravityScale returns the current gravity scale
// (upstream b2Body_GetGravityScale).
func (w *World) BodyGravityScale(bodyID BodyID) float64 {
	assert(w.IsBodyValid(bodyID))
	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	return bSim.gravityScale
}

// IsBodyAwake reports whether this body is awake (upstream b2Body_IsAwake).
func (w *World) IsBodyAwake(bodyID BodyID) bool {
	b := w.getBodyFullID(bodyID)
	return b.setIndex == awakeSet
}

// SetBodyAwake wakes a body from sleep, or puts it to sleep. Putting a body
// to sleep will put the entire island of bodies touching this body to sleep,
// which can be expensive and possibly unintuitive
// (upstream b2Body_SetAwake).
func (w *World) SetBodyAwake(bodyID BodyID, awake bool) {
	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)

	if awake && b.setIndex >= firstSleepingSet {
		w.wakeBody(b)
	} else if !awake && b.setIndex == awakeSet {
		isl := &w.islands[b.islandID]
		if isl.constraintRemoveCount > 0 {
			// Must split the island before sleeping. This is expensive.
			w.splitIsland(b.islandID)
		}

		w.trySleepIsland(b.islandID)
	}
}

// WakeBodyTouching wakes all the bodies touching this body via contacts
// (upstream b2Body_WakeTouching).
func (w *World) WakeBodyTouching(bodyID BodyID) {
	b := w.getBodyFullID(bodyID)

	contactKey := b.headContactKey
	for contactKey != NullIndex {
		contactID := contactKey >> 1
		edgeIndex := contactKey & 1

		c := &w.contacts[contactID]
		shapeA := &w.shapes[c.shapeIDA]
		shapeB := &w.shapes[c.shapeIDB]

		if shapeA.bodyID == int(bodyID.index1)-1 {
			otherBody := &w.bodies[shapeB.bodyID]
			w.wakeBody(otherBody)
		} else {
			otherBody := &w.bodies[shapeA.bodyID]
			w.wakeBody(otherBody)
		}

		contactKey = c.edges[edgeIndex].nextKey
	}
}

// IsBodyEnabled reports whether this body is enabled
// (upstream b2Body_IsEnabled).
func (w *World) IsBodyEnabled(bodyID BodyID) bool {
	b := w.getBodyFullID(bodyID)
	return b.setIndex != disabledSet
}

// IsBodySleepEnabled reports whether this body can fall asleep
// (upstream b2Body_IsSleepEnabled).
func (w *World) IsBodySleepEnabled(bodyID BodyID) bool {
	b := w.getBodyFullID(bodyID)
	return b.enableSleep
}

// SetBodySleepThreshold sets the sleep threshold, usually in meters per
// second (upstream b2Body_SetSleepThreshold).
func (w *World) SetBodySleepThreshold(bodyID BodyID, sleepThreshold float64) {
	b := w.getBodyFullID(bodyID)
	b.sleepThreshold = sleepThreshold
}

// BodySleepThreshold returns the sleep threshold, usually in meters per
// second (upstream b2Body_GetSleepThreshold).
func (w *World) BodySleepThreshold(bodyID BodyID) float64 {
	b := w.getBodyFullID(bodyID)
	return b.sleepThreshold
}

// EnableBodySleep enables or disables sleeping for this body. If sleep is
// disabled the body will wake (upstream b2Body_EnableSleep).
func (w *World) EnableBodySleep(bodyID BodyID, enableSleep bool) {
	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	b.enableSleep = enableSleep

	if !enableSleep {
		w.wakeBody(b)
	}
}

// DisableBody disables a body by removing it completely from the simulation.
// This is expensive (upstream b2Body_Disable).
//
// Disabling a body requires a lot of detailed bookkeeping, but it is a
// valuable feature. The most challenging aspect is that joints may connect to
// bodies that are not disabled.
func (w *World) DisableBody(bodyID BodyID) {
	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	if b.setIndex == disabledSet {
		return
	}

	// Destroy contacts and wake bodies touching this body. This avoid floating
	// bodies. This is necessary even for static bodies.
	wakeBodies := true
	w.destroyBodyContacts(b, wakeBodies)

	// The current solver set of the body
	set := &w.solverSets[b.setIndex]

	// Disabled bodies and connected joints are moved to the disabled set
	disabled := &w.solverSets[disabledSet]

	// Unlink joints and transfer them to the disabled set
	jointKey := b.headJointKey
	for jointKey != NullIndex {
		jointID := jointKey >> 1
		edgeIndex := jointKey & 1

		j := &w.joints[jointID]
		jointKey = j.edges[edgeIndex].nextKey

		// joint may already be disabled by other body
		if j.setIndex == disabledSet {
			continue
		}

		assert(j.setIndex == set.setIndex || set.setIndex == staticSet)

		// Remove joint from island
		w.unlinkJoint(j)

		// Transfer joint to disabled set
		jointSet := &w.solverSets[j.setIndex]
		w.transferJoint(disabled, jointSet, j)
	}

	// Remove shapes from broad-phase
	shapeID := b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]
		shapeID = s.nextShapeID
		destroyShapeProxy(s, &w.broadPhase)
	}

	// Disabled bodies are not in an island. If the island becomes empty it
	// will be destroyed.
	w.removeBodyFromIsland(b)

	// Transfer body sim
	w.transferBody(disabled, set, b)

	w.validateConnectivity()
	w.validateSolverSets()
}

// EnableBody enables a body by adding it to the simulation. This is expensive
// (upstream b2Body_Enable).
func (w *World) EnableBody(bodyID BodyID) {
	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	if b.setIndex != disabledSet {
		return
	}

	disabled := &w.solverSets[disabledSet]
	setID := awakeSet
	if b.bodyType == StaticBody {
		setID = staticSet
	}
	targetSet := &w.solverSets[setID]

	w.transferBody(targetSet, disabled, b)

	transform := w.getBodyTransformQuick(b)

	// Add shapes to broad-phase
	proxyType := b.bodyType
	forcePairCreation := true
	shapeID := b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]
		shapeID = s.nextShapeID

		createShapeProxy(s, &w.broadPhase, proxyType, transform, forcePairCreation)
	}

	if setID != staticSet {
		w.createIslandForBody(setID, b)
	}

	// Transfer joints. If the other body is disabled, don't transfer.
	// If the other body is sleeping, wake it.
	jointKey := b.headJointKey
	for jointKey != NullIndex {
		jointID := jointKey >> 1
		edgeIndex := jointKey & 1

		j := &w.joints[jointID]
		assert(j.setIndex == disabledSet)
		assert(j.islandID == NullIndex)

		jointKey = j.edges[edgeIndex].nextKey

		bodyA := &w.bodies[j.edges[0].bodyID]
		bodyB := &w.bodies[j.edges[1].bodyID]

		if bodyA.setIndex == disabledSet || bodyB.setIndex == disabledSet {
			// one body is still disabled
			continue
		}

		// Transfer joint first
		var jointSetID int
		switch {
		case bodyA.setIndex == staticSet && bodyB.setIndex == staticSet:
			jointSetID = staticSet
		case bodyA.setIndex == staticSet:
			jointSetID = bodyB.setIndex
		default:
			jointSetID = bodyA.setIndex
		}

		jointSet := &w.solverSets[jointSetID]
		w.transferJoint(jointSet, disabled, j)

		// Now that the joint is in the correct set, it can be linked in the
		// island.
		if jointSetID != staticSet {
			w.linkJoint(j)
		}
	}

	w.validateSolverSets()
}

// SetBodyMotionLocks sets the motion locks on this body
// (upstream b2Body_SetMotionLocks).
func (w *World) SetBodyMotionLocks(bodyID BodyID, locks MotionLocks) {
	if w.locked {
		assert(false)
		return
	}

	var newFlags uint32
	if locks.LinearX {
		newFlags |= lockLinearX
	}
	if locks.LinearY {
		newFlags |= lockLinearY
	}
	if locks.AngularZ {
		newFlags |= lockAngularZ
	}

	b := w.getBodyFullID(bodyID)
	if b.flags&allLocks != newFlags {
		b.flags &^= allLocks
		b.flags |= newFlags

		bSim := w.getBodySim(b)
		bSim.flags &^= allLocks
		bSim.flags |= newFlags

		state := w.getBodyState(b)

		if state != nil {
			state.flags = bSim.flags

			if locks.LinearX {
				state.linearVelocity.X = 0.0
			}

			if locks.LinearY {
				state.linearVelocity.Y = 0.0
			}

			if locks.AngularZ {
				state.angularVelocity = 0.0
			}
		}
	}
}

// BodyMotionLocks returns the motion locks on this body
// (upstream b2Body_GetMotionLocks).
func (w *World) BodyMotionLocks(bodyID BodyID) MotionLocks {
	b := w.getBodyFullID(bodyID)

	return MotionLocks{
		LinearX:  b.flags&lockLinearX != 0,
		LinearY:  b.flags&lockLinearY != 0,
		AngularZ: b.flags&lockAngularZ != 0,
	}
}

// SetBodyBullet sets this body to be a bullet. A bullet does continuous
// collision detection against dynamic bodies (but not other bullets)
// (upstream b2Body_SetBullet).
func (w *World) SetBodyBullet(bodyID BodyID, flag bool) {
	if w.locked {
		assert(false)
		return
	}

	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)

	if flag {
		bSim.flags |= isBullet
	} else {
		bSim.flags &^= isBullet
	}
}

// IsBodyBullet reports whether this body is a bullet
// (upstream b2Body_IsBullet).
func (w *World) IsBodyBullet(bodyID BodyID) bool {
	b := w.getBodyFullID(bodyID)
	bSim := w.getBodySim(b)
	return bSim.flags&isBullet != 0
}

// EnableBodyContactEvents enables/disables contact events on all shapes
// (upstream b2Body_EnableContactEvents).
//
// Warning: changing this at runtime may cause mismatched begin/end touch
// events.
func (w *World) EnableBodyContactEvents(bodyID BodyID, flag bool) {
	b := w.getBodyFullID(bodyID)
	shapeID := b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]
		s.enableContactEvents = flag
		shapeID = s.nextShapeID
	}
}

// EnableBodyHitEvents enables/disables hit events on all shapes
// (upstream b2Body_EnableHitEvents).
func (w *World) EnableBodyHitEvents(bodyID BodyID, flag bool) {
	b := w.getBodyFullID(bodyID)
	shapeID := b.headShapeID
	for shapeID != NullIndex {
		s := &w.shapes[shapeID]
		s.enableHitEvents = flag
		shapeID = s.nextShapeID
	}
}

// BodyShapeCount returns the number of shapes on this body
// (upstream b2Body_GetShapeCount).
func (w *World) BodyShapeCount(bodyID BodyID) int {
	b := w.getBodyFullID(bodyID)
	return b.shapeCount
}

// BodyShapes fills shapeArray with the shape ids for all shapes on this body,
// up to len(shapeArray), and returns the number of ids stored
// (upstream b2Body_GetShapes).
func (w *World) BodyShapes(bodyID BodyID, shapeArray []ShapeID) int {
	b := w.getBodyFullID(bodyID)
	shapeID := b.headShapeID
	shapeCount := 0
	for shapeID != NullIndex && shapeCount < len(shapeArray) {
		s := &w.shapes[shapeID]
		shapeArray[shapeCount] = ShapeID{index1: int32(s.id + 1), world0: bodyID.world0, generation: s.generation}
		shapeCount++

		shapeID = s.nextShapeID
	}

	return shapeCount
}

// BodyJointCount returns the number of joints on this body
// (upstream b2Body_GetJointCount).
func (w *World) BodyJointCount(bodyID BodyID) int {
	b := w.getBodyFullID(bodyID)
	return b.jointCount
}

// BodyJoints fills jointArray with the joint ids for all joints on this body,
// up to len(jointArray), and returns the number of ids stored
// (upstream b2Body_GetJoints).
func (w *World) BodyJoints(bodyID BodyID, jointArray []JointID) int {
	b := w.getBodyFullID(bodyID)
	jointKey := b.headJointKey

	jointCount := 0
	for jointKey != NullIndex && jointCount < len(jointArray) {
		jointID := jointKey >> 1
		edgeIndex := jointKey & 1

		j := &w.joints[jointID]

		jointArray[jointCount] = JointID{index1: int32(jointID + 1), world0: bodyID.world0, generation: j.generation}
		jointCount++

		jointKey = j.edges[edgeIndex].nextKey
	}

	return jointCount
}

// shouldBodiesCollide reports whether two bodies should collide, considering
// joints with collideConnected == false (upstream b2ShouldBodiesCollide).
func (w *World) shouldBodiesCollide(bodyA, bodyB *body) bool {
	if bodyA.bodyType != DynamicBody && bodyB.bodyType != DynamicBody {
		return false
	}

	var jointKey int
	var otherBodyID int
	if bodyA.jointCount < bodyB.jointCount {
		jointKey = bodyA.headJointKey
		otherBodyID = bodyB.id
	} else {
		jointKey = bodyB.headJointKey
		otherBodyID = bodyA.id
	}

	for jointKey != NullIndex {
		jointID := jointKey >> 1
		edgeIndex := jointKey & 1
		otherEdgeIndex := edgeIndex ^ 1

		j := &w.joints[jointID]
		if !j.collideConnected && j.edges[otherEdgeIndex].bodyID == otherBodyID {
			return false
		}

		jointKey = j.edges[edgeIndex].nextKey
	}

	return true
}
