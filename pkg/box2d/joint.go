// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/joint.h, src/joint.c.
//
// Public API mapping: b2CreateDistanceJoint → (*World).CreateDistanceJoint,
// b2Joint_GetType → (*World).JointType, b2Joint_SetLocalFrameA →
// (*World).SetJointLocalFrameA, and so on (getters are Get-less).
//
// Deviations from upstream:
//   - b2JointSim ends with a union of the per-type joint structs. Go has no
//     unions; the per-type structs are separate fields (only one is live at a
//     time, selected by jointType), mirroring the approach used for the shape
//     geometry union in shape.go.
//   - b2Joint_GetWorld is not ported: there is no world registry, callers
//     already hold the *World (see world.go).
//   - b2DrawJoint and the per-type draw functions are ported below.

package box2d

import (
	"math"
	"strconv"
)

// jointEdge connects bodies and joints together in a joint graph where each
// body is a node and each joint is an edge (upstream b2JointEdge). A joint
// edge belongs to a doubly linked list maintained in each attached body. Each
// joint has two joint edges, one for each attached body.
type jointEdge struct {
	bodyID  int
	prevKey int
	nextKey int
}

// joint maps from JointID to the joint data in the solver sets
// (upstream b2Joint).
type joint struct {
	// User data. Deviation from upstream: the C void* becomes a uint64 so the
	// ECS wrapper can pack an entity id directly.
	userData uint64

	// index of simulation set stored in World; NullIndex when the slot is free
	setIndex int

	// index into the constraint graph color array; may be NullIndex for
	// sleeping/disabled joints; NullIndex when the slot is free
	colorIndex int

	// joint index within set or graph color; NullIndex when the slot is free
	localIndex int

	edges [2]jointEdge

	jointID  int
	islandID int

	// Index into the island's joints array for O(1) swap-removal.
	// NullIndex when not in an island.
	islandIndex int

	drawScale float64

	jointType JointType

	// This is monotonically advanced when a joint is allocated in this slot.
	// Used to check for invalid JointID.
	generation uint16

	collideConnected bool
}

// jointSim is the base joint class used by the constraint solver
// (upstream b2JointSim).
type jointSim struct {
	jointID int

	bodyIDA int
	bodyIDB int

	jointType JointType

	localFrameA Transform
	localFrameB Transform

	invMassA float64
	invMassB float64
	invIA    float64
	invIB    float64

	constraintHertz        float64
	constraintDampingRatio float64

	constraintSoftness softness

	forceThreshold  float64
	torqueThreshold float64

	// Per-type joint state. Deviation from upstream: the C union becomes
	// separate fields; only the field matching jointType is live. The filter
	// joint has no per-type state.
	distanceJoint  distanceJoint
	motorJoint     motorJoint
	revoluteJoint  revoluteJoint
	prismaticJoint prismaticJoint
	weldJoint      weldJoint
	wheelJoint     wheelJoint
}

// jointPair pairs the joint bookkeeping struct with its sim
// (upstream b2JointPair).
type jointPair struct {
	joint    *joint
	jointSim *jointSim
}

// ---------------------------------------------------------------------------
// Default joint definitions (upstream b2Default*JointDef in joint.c)
// ---------------------------------------------------------------------------

// defaultJointDef initializes the shared base joint definition
// (upstream static b2DefaultJointDef).
func defaultJointDef() JointDef {
	var def JointDef
	def.LocalFrameA.Q = RotIdentity
	def.LocalFrameB.Q = RotIdentity
	// Keep the upstream FLT_MAX magnitude even though this port computes in
	// float64: the joint event scan treats a threshold below this value as
	// "events enabled".
	def.ForceThreshold = math.MaxFloat32
	def.TorqueThreshold = math.MaxFloat32
	def.ConstraintHertz = 60.0
	def.ConstraintDampingRatio = 2.0
	def.DrawScale = GetLengthUnitsPerMeter()
	return def
}

// DefaultDistanceJointDef initializes a distance joint definition
// (upstream b2DefaultDistanceJointDef).
func DefaultDistanceJointDef() DistanceJointDef {
	var def DistanceJointDef
	def.Base = defaultJointDef()
	def.LowerSpringForce = -math.MaxFloat32
	def.UpperSpringForce = math.MaxFloat32
	def.Length = 1.0
	def.MaxLength = Huge
	def.initialized = true
	return def
}

// DefaultMotorJointDef initializes a motor joint definition
// (upstream b2DefaultMotorJointDef).
func DefaultMotorJointDef() MotorJointDef {
	var def MotorJointDef
	def.Base = defaultJointDef()
	def.initialized = true
	return def
}

// DefaultFilterJointDef initializes a filter joint definition
// (upstream b2DefaultFilterJointDef).
func DefaultFilterJointDef() FilterJointDef {
	var def FilterJointDef
	def.Base = defaultJointDef()
	def.initialized = true
	return def
}

// DefaultPrismaticJointDef initializes a prismatic joint definition
// (upstream b2DefaultPrismaticJointDef).
func DefaultPrismaticJointDef() PrismaticJointDef {
	var def PrismaticJointDef
	def.Base = defaultJointDef()
	def.initialized = true
	return def
}

// DefaultRevoluteJointDef initializes a revolute joint definition
// (upstream b2DefaultRevoluteJointDef).
func DefaultRevoluteJointDef() RevoluteJointDef {
	var def RevoluteJointDef
	def.Base = defaultJointDef()
	def.initialized = true
	return def
}

// DefaultWeldJointDef initializes a weld joint definition
// (upstream b2DefaultWeldJointDef).
func DefaultWeldJointDef() WeldJointDef {
	var def WeldJointDef
	def.Base = defaultJointDef()
	def.initialized = true
	return def
}

// DefaultWheelJointDef initializes a wheel joint definition
// (upstream b2DefaultWheelJointDef).
func DefaultWheelJointDef() WheelJointDef {
	var def WheelJointDef
	def.Base = defaultJointDef()
	def.EnableSpring = true
	def.Hertz = 1.0
	def.DampingRatio = 0.7
	def.initialized = true
	return def
}

// DefaultExplosionDef initializes an explosion definition
// (upstream b2DefaultExplosionDef, which lives in joint.c).
func DefaultExplosionDef() ExplosionDef {
	return ExplosionDef{MaskBits: DefaultMaskBits}
}

// ---------------------------------------------------------------------------
// Lookup helpers
// ---------------------------------------------------------------------------

// getJointFullID returns a validated joint from a JointID
// (upstream b2GetJointFullId).
func (w *World) getJointFullID(jointID JointID) *joint {
	id := int(jointID.index1) - 1
	j := &w.joints[id]
	assert(w.ownsToken(jointID.world0) && j.jointID == id && j.generation == jointID.generation)
	return j
}

// getJointSim returns the joint sim for a joint, looking in the constraint
// graph for awake joints (upstream b2GetJointSim).
func (w *World) getJointSim(j *joint) *jointSim {
	if j.setIndex == awakeSet {
		assert(0 <= j.colorIndex && j.colorIndex < GraphColorCount)
		color := &w.constraintGraph.colors[j.colorIndex]
		return &color.jointSims[j.localIndex]
	}

	set := &w.solverSets[j.setIndex]
	return &set.jointSims[j.localIndex]
}

// getJointSimCheckType returns the joint sim for a joint id, asserting the
// joint type (upstream b2GetJointSimCheckType). Returns nil when the world is
// locked, matching the upstream NULL return.
//
// Every public joint accessor resolves through here — with exactly one upstream
// exemption: b2PrismaticJoint_GetSpeed resolves the sim itself (b2GetWorld +
// b2GetJointSim) and so keeps working while the world is locked, i.e. when
// called from a step-time user callback. PrismaticJointSpeed reproduces that,
// deliberately; routing it through this helper instead returns nil here and
// panics at the caller's first field access. Do not "fix" that asymmetry
// without checking upstream first (see the note on PrismaticJointSpeed in
// prismatic_joint.go).
func (w *World) getJointSimCheckType(jointID JointID, jointType JointType) *jointSim {
	assert(!w.locked)
	if w.locked {
		return nil
	}

	j := w.getJointFullID(jointID)
	assert(j.jointType == jointType)
	js := w.getJointSim(j)
	assert(js.jointType == jointType)
	return js
}

// destroyContactsBetweenBodies destroys the contacts between two bodies,
// used when a joint disables collision between the attached bodies
// (upstream static b2DestroyContactsBetweenBodies).
func (w *World) destroyContactsBetweenBodies(bodyA, bodyB *body) {
	var contactKey int
	var otherBodyID int

	// use the smaller of the two contact lists
	if bodyA.contactCount < bodyB.contactCount {
		contactKey = bodyA.headContactKey
		otherBodyID = bodyB.id
	} else {
		contactKey = bodyB.headContactKey
		otherBodyID = bodyA.id
	}

	// no need to wake bodies when a joint removes collision between them
	wakeBodies := false

	// destroy the contacts
	for contactKey != NullIndex {
		contactID := contactKey >> 1
		edgeIndex := contactKey & 1

		c := &w.contacts[contactID]
		contactKey = c.edges[edgeIndex].nextKey

		otherEdgeIndex := edgeIndex ^ 1
		if c.edges[otherEdgeIndex].bodyID == otherBodyID {
			// Careful, this removes the contact from the current doubly linked
			// list
			w.destroyContact(c, wakeBodies)
		}
	}

	w.validateSolverSets()
}

// ---------------------------------------------------------------------------
// Joint creation
// ---------------------------------------------------------------------------

// createJointInternal creates the joint bookkeeping and sim for any joint
// type (upstream static b2CreateJoint).
func (w *World) createJointInternal(def *JointDef, jointType JointType) jointPair {
	assert(IsValidTransform(def.LocalFrameA))
	assert(IsValidTransform(def.LocalFrameB))
	assert(w.worldID == def.BodyIDA.world0)
	assert(w.worldID == def.BodyIDB.world0)
	assert(def.BodyIDA != def.BodyIDB)

	bodyA := w.getBodyFullID(def.BodyIDA)
	bodyB := w.getBodyFullID(def.BodyIDB)

	bodyIDA := bodyA.id
	bodyIDB := bodyB.id
	maxSetIndex := maxInt(bodyA.setIndex, bodyB.setIndex)

	// Create joint id and joint
	jointID := allocID(&w.jointIDPool)
	if jointID == len(w.joints) {
		w.joints = append(w.joints, joint{})
	}

	j := &w.joints[jointID]
	j.jointID = jointID
	j.userData = def.UserData
	j.generation++
	j.setIndex = NullIndex
	j.colorIndex = NullIndex
	j.localIndex = NullIndex
	j.islandID = NullIndex
	j.islandIndex = NullIndex
	j.drawScale = def.DrawScale
	j.jointType = jointType
	j.collideConnected = def.CollideConnected

	// Doubly linked list on bodyA
	j.edges[0].bodyID = bodyIDA
	j.edges[0].prevKey = NullIndex
	j.edges[0].nextKey = bodyA.headJointKey

	keyA := jointID << 1 // | 0 for edge index 0
	if bodyA.headJointKey != NullIndex {
		jointA := &w.joints[bodyA.headJointKey>>1]
		edgeA := &jointA.edges[bodyA.headJointKey&1]
		edgeA.prevKey = keyA
	}
	bodyA.headJointKey = keyA
	bodyA.jointCount++

	// Doubly linked list on bodyB
	j.edges[1].bodyID = bodyIDB
	j.edges[1].prevKey = NullIndex
	j.edges[1].nextKey = bodyB.headJointKey

	keyB := (jointID << 1) | 1
	if bodyB.headJointKey != NullIndex {
		jointB := &w.joints[bodyB.headJointKey>>1]
		edgeB := &jointB.edges[bodyB.headJointKey&1]
		edgeB.prevKey = keyB
	}
	bodyB.headJointKey = keyB
	bodyB.jointCount++

	var js *jointSim

	switch {
	case bodyA.setIndex == disabledSet || bodyB.setIndex == disabledSet:
		// if either body is disabled, create in disabled set
		set := &w.solverSets[disabledSet]
		j.setIndex = disabledSet
		j.localIndex = len(set.jointSims)

		set.jointSims = append(set.jointSims, jointSim{})
		js = &set.jointSims[len(set.jointSims)-1]

		js.jointID = jointID
		js.bodyIDA = bodyIDA
		js.bodyIDB = bodyIDB

	case bodyA.bodyType != DynamicBody && bodyB.bodyType != DynamicBody:
		// joint is not attached to a dynamic body
		set := &w.solverSets[staticSet]
		j.setIndex = staticSet
		j.localIndex = len(set.jointSims)

		set.jointSims = append(set.jointSims, jointSim{})
		js = &set.jointSims[len(set.jointSims)-1]

		js.jointID = jointID
		js.bodyIDA = bodyIDA
		js.bodyIDB = bodyIDB

	case bodyA.setIndex == awakeSet || bodyB.setIndex == awakeSet:
		// if either body is sleeping, wake it
		if maxSetIndex >= firstSleepingSet {
			w.wakeSolverSet(maxSetIndex)
		}

		j.setIndex = awakeSet

		js = w.createJointInGraph(j)
		js.jointID = jointID
		js.bodyIDA = bodyIDA
		js.bodyIDB = bodyIDB

	default:
		// joint connected between sleeping and/or static bodies
		assert(bodyA.setIndex >= firstSleepingSet || bodyB.setIndex >= firstSleepingSet)
		assert(bodyA.setIndex != staticSet || bodyB.setIndex != staticSet)

		// joint should go into the sleeping set (not static set)
		setIndex := maxSetIndex

		set := &w.solverSets[setIndex]
		j.setIndex = setIndex
		j.localIndex = len(set.jointSims)

		set.jointSims = append(set.jointSims, jointSim{})
		js = &set.jointSims[len(set.jointSims)-1]

		// These must be set to accommodate the merge below
		js.jointID = jointID
		js.bodyIDA = bodyIDA
		js.bodyIDB = bodyIDB

		if bodyA.setIndex != bodyB.setIndex && bodyA.setIndex >= firstSleepingSet &&
			bodyB.setIndex >= firstSleepingSet {
			// merge sleeping sets
			w.mergeSolverSets(bodyA.setIndex, bodyB.setIndex)
			assert(bodyA.setIndex == bodyB.setIndex)

			// fix potentially invalid set index
			setIndex = bodyA.setIndex

			mergedSet := &w.solverSets[setIndex]

			// Careful! The joint sim pointer was orphaned by the set merge.
			js = &mergedSet.jointSims[j.localIndex]
		}

		assert(j.setIndex == setIndex)
	}

	js.localFrameA = def.LocalFrameA
	js.localFrameB = def.LocalFrameB
	js.jointType = jointType
	js.constraintHertz = def.ConstraintHertz
	js.constraintDampingRatio = def.ConstraintDampingRatio
	js.constraintSoftness = softness{
		biasRate:     0.0,
		massScale:    1.0,
		impulseScale: 0.0,
	}

	assert(IsValidFloat(def.ForceThreshold) && def.ForceThreshold >= 0.0)
	assert(IsValidFloat(def.TorqueThreshold) && def.TorqueThreshold >= 0.0)

	js.forceThreshold = def.ForceThreshold
	js.torqueThreshold = def.TorqueThreshold

	assert(js.jointID == jointID)
	assert(js.bodyIDA == bodyIDA)
	assert(js.bodyIDB == bodyIDB)

	if j.setIndex > disabledSet {
		// Add edge to island graph
		w.linkJoint(j)
	}

	// If the joint prevents collisions, then destroy all contacts between
	// attached bodies
	if !def.CollideConnected {
		w.destroyContactsBetweenBodies(bodyA, bodyB)
	}

	w.validateSolverSets()

	return jointPair{joint: j, jointSim: js}
}

// CreateDistanceJoint creates a distance joint (upstream
// b2CreateDistanceJoint).
func (w *World) CreateDistanceJoint(def *DistanceJointDef) JointID {
	requireInitialized(def.initialized, "DistanceJointDef", "DefaultDistanceJointDef")

	assert(!w.locked)
	if w.locked {
		return JointID{}
	}

	assert(IsValidFloat(def.Length) && def.Length > 0.0)
	assert(def.LowerSpringForce <= def.UpperSpringForce)

	pair := w.createJointInternal(&def.Base, DistanceJoint)

	js := pair.jointSim

	js.distanceJoint = distanceJoint{}
	js.distanceJoint.length = maxFloat(def.Length, LinearSlop)
	js.distanceJoint.hertz = def.Hertz
	js.distanceJoint.dampingRatio = def.DampingRatio
	js.distanceJoint.minLength = maxFloat(def.MinLength, LinearSlop)
	js.distanceJoint.maxLength = maxFloat(def.MinLength, def.MaxLength)
	js.distanceJoint.maxMotorForce = def.MaxMotorForce
	js.distanceJoint.motorSpeed = def.MotorSpeed
	js.distanceJoint.enableSpring = def.EnableSpring
	js.distanceJoint.lowerSpringForce = def.LowerSpringForce
	js.distanceJoint.upperSpringForce = def.UpperSpringForce
	js.distanceJoint.enableLimit = def.EnableLimit
	js.distanceJoint.enableMotor = def.EnableMotor
	js.distanceJoint.impulse = 0.0
	js.distanceJoint.lowerImpulse = 0.0
	js.distanceJoint.upperImpulse = 0.0
	js.distanceJoint.motorImpulse = 0.0

	return JointID{index1: int32(js.jointID + 1), world0: w.worldID, generation: pair.joint.generation}
}

// CreateMotorJoint creates a motor joint (upstream b2CreateMotorJoint).
func (w *World) CreateMotorJoint(def *MotorJointDef) JointID {
	requireInitialized(def.initialized, "MotorJointDef", "DefaultMotorJointDef")

	assert(!w.locked)
	if w.locked {
		return JointID{}
	}

	pair := w.createJointInternal(&def.Base, MotorJoint)
	js := pair.jointSim

	js.motorJoint = motorJoint{}
	js.motorJoint.linearVelocity = def.LinearVelocity
	js.motorJoint.maxVelocityForce = def.MaxVelocityForce
	js.motorJoint.angularVelocity = def.AngularVelocity
	js.motorJoint.maxVelocityTorque = def.MaxVelocityTorque
	js.motorJoint.linearHertz = def.LinearHertz
	js.motorJoint.linearDampingRatio = def.LinearDampingRatio
	js.motorJoint.maxSpringForce = def.MaxSpringForce
	js.motorJoint.angularHertz = def.AngularHertz
	js.motorJoint.angularDampingRatio = def.AngularDampingRatio
	js.motorJoint.maxSpringTorque = def.MaxSpringTorque

	return JointID{index1: int32(js.jointID + 1), world0: w.worldID, generation: pair.joint.generation}
}

// CreateFilterJoint creates a filter joint. A filter joint disables collision
// between the attached bodies and has no solve work
// (upstream b2CreateFilterJoint).
func (w *World) CreateFilterJoint(def *FilterJointDef) JointID {
	requireInitialized(def.initialized, "FilterJointDef", "DefaultFilterJointDef")

	assert(!w.locked)
	if w.locked {
		return JointID{}
	}

	pair := w.createJointInternal(&def.Base, FilterJoint)

	js := pair.jointSim

	return JointID{index1: int32(js.jointID + 1), world0: w.worldID, generation: pair.joint.generation}
}

// CreatePrismaticJoint creates a prismatic (slider) joint
// (upstream b2CreatePrismaticJoint).
func (w *World) CreatePrismaticJoint(def *PrismaticJointDef) JointID {
	requireInitialized(def.initialized, "PrismaticJointDef", "DefaultPrismaticJointDef")
	assert(def.LowerTranslation <= def.UpperTranslation)

	assert(!w.locked)
	if w.locked {
		return JointID{}
	}

	pair := w.createJointInternal(&def.Base, PrismaticJoint)

	js := pair.jointSim

	js.prismaticJoint = prismaticJoint{}
	js.prismaticJoint.hertz = def.Hertz
	js.prismaticJoint.dampingRatio = def.DampingRatio
	js.prismaticJoint.targetTranslation = def.TargetTranslation
	js.prismaticJoint.lowerTranslation = def.LowerTranslation
	js.prismaticJoint.upperTranslation = def.UpperTranslation
	js.prismaticJoint.maxMotorForce = def.MaxMotorForce
	js.prismaticJoint.motorSpeed = def.MotorSpeed
	js.prismaticJoint.enableSpring = def.EnableSpring
	js.prismaticJoint.enableLimit = def.EnableLimit
	js.prismaticJoint.enableMotor = def.EnableMotor

	return JointID{index1: int32(js.jointID + 1), world0: w.worldID, generation: pair.joint.generation}
}

// CreateRevoluteJoint creates a revolute (hinge) joint
// (upstream b2CreateRevoluteJoint).
func (w *World) CreateRevoluteJoint(def *RevoluteJointDef) JointID {
	requireInitialized(def.initialized, "RevoluteJointDef", "DefaultRevoluteJointDef")
	assert(def.LowerAngle <= def.UpperAngle)
	assert(def.LowerAngle >= -0.99*Pi)
	assert(def.UpperAngle <= 0.99*Pi)

	assert(!w.locked)
	if w.locked {
		return JointID{}
	}

	pair := w.createJointInternal(&def.Base, RevoluteJoint)

	js := pair.jointSim

	js.revoluteJoint = revoluteJoint{}

	js.revoluteJoint.targetAngle = clampFloat(def.TargetAngle, -Pi, Pi)
	js.revoluteJoint.hertz = def.Hertz
	js.revoluteJoint.dampingRatio = def.DampingRatio
	js.revoluteJoint.lowerAngle = def.LowerAngle
	js.revoluteJoint.upperAngle = def.UpperAngle
	js.revoluteJoint.maxMotorTorque = def.MaxMotorTorque
	js.revoluteJoint.motorSpeed = def.MotorSpeed
	js.revoluteJoint.enableSpring = def.EnableSpring
	js.revoluteJoint.enableLimit = def.EnableLimit
	js.revoluteJoint.enableMotor = def.EnableMotor

	return JointID{index1: int32(js.jointID + 1), world0: w.worldID, generation: pair.joint.generation}
}

// CreateWeldJoint creates a weld joint (upstream b2CreateWeldJoint).
func (w *World) CreateWeldJoint(def *WeldJointDef) JointID {
	requireInitialized(def.initialized, "WeldJointDef", "DefaultWeldJointDef")

	assert(!w.locked)
	if w.locked {
		return JointID{}
	}

	pair := w.createJointInternal(&def.Base, WeldJoint)

	js := pair.jointSim

	js.weldJoint = weldJoint{}
	js.weldJoint.linearHertz = def.LinearHertz
	js.weldJoint.linearDampingRatio = def.LinearDampingRatio
	js.weldJoint.angularHertz = def.AngularHertz
	js.weldJoint.angularDampingRatio = def.AngularDampingRatio
	js.weldJoint.linearImpulse = Vec2Zero
	js.weldJoint.angularImpulse = 0.0

	return JointID{index1: int32(js.jointID + 1), world0: w.worldID, generation: pair.joint.generation}
}

// CreateWheelJoint creates a wheel joint (upstream b2CreateWheelJoint).
func (w *World) CreateWheelJoint(def *WheelJointDef) JointID {
	requireInitialized(def.initialized, "WheelJointDef", "DefaultWheelJointDef")
	assert(def.LowerTranslation <= def.UpperTranslation)

	assert(!w.locked)
	if w.locked {
		return JointID{}
	}

	pair := w.createJointInternal(&def.Base, WheelJoint)

	js := pair.jointSim

	js.wheelJoint = wheelJoint{}
	js.wheelJoint.perpMass = 0.0
	js.wheelJoint.axialMass = 0.0
	js.wheelJoint.motorImpulse = 0.0
	js.wheelJoint.lowerImpulse = 0.0
	js.wheelJoint.upperImpulse = 0.0
	js.wheelJoint.lowerTranslation = def.LowerTranslation
	js.wheelJoint.upperTranslation = def.UpperTranslation
	js.wheelJoint.maxMotorTorque = def.MaxMotorTorque
	js.wheelJoint.motorSpeed = def.MotorSpeed
	js.wheelJoint.hertz = def.Hertz
	js.wheelJoint.dampingRatio = def.DampingRatio
	js.wheelJoint.enableSpring = def.EnableSpring
	js.wheelJoint.enableLimit = def.EnableLimit
	js.wheelJoint.enableMotor = def.EnableMotor

	return JointID{index1: int32(js.jointID + 1), world0: w.worldID, generation: pair.joint.generation}
}

// ---------------------------------------------------------------------------
// Joint destruction
// ---------------------------------------------------------------------------

// destroyJointInternal removes a joint from bodies, islands, the constraint
// graph or its solver set, and frees the joint id
// (upstream b2DestroyJointInternal).
func (w *World) destroyJointInternal(j *joint, wakeBodies bool) {
	jointID := j.jointID

	edgeA := &j.edges[0]
	edgeB := &j.edges[1]

	idA := edgeA.bodyID
	idB := edgeB.bodyID
	bodyA := &w.bodies[idA]
	bodyB := &w.bodies[idB]

	// Remove from body A
	if edgeA.prevKey != NullIndex {
		prevJoint := &w.joints[edgeA.prevKey>>1]
		prevEdge := &prevJoint.edges[edgeA.prevKey&1]
		prevEdge.nextKey = edgeA.nextKey
	}

	if edgeA.nextKey != NullIndex {
		nextJoint := &w.joints[edgeA.nextKey>>1]
		nextEdge := &nextJoint.edges[edgeA.nextKey&1]
		nextEdge.prevKey = edgeA.prevKey
	}

	edgeKeyA := jointID << 1 // | 0 for edge index 0
	if bodyA.headJointKey == edgeKeyA {
		bodyA.headJointKey = edgeA.nextKey
	}

	bodyA.jointCount--

	// Remove from body B
	if edgeB.prevKey != NullIndex {
		prevJoint := &w.joints[edgeB.prevKey>>1]
		prevEdge := &prevJoint.edges[edgeB.prevKey&1]
		prevEdge.nextKey = edgeB.nextKey
	}

	if edgeB.nextKey != NullIndex {
		nextJoint := &w.joints[edgeB.nextKey>>1]
		nextEdge := &nextJoint.edges[edgeB.nextKey&1]
		nextEdge.prevKey = edgeB.prevKey
	}

	edgeKeyB := (jointID << 1) | 1
	if bodyB.headJointKey == edgeKeyB {
		bodyB.headJointKey = edgeB.nextKey
	}

	bodyB.jointCount--

	if j.islandID != NullIndex {
		assert(j.setIndex > disabledSet)
		w.unlinkJoint(j)
	} else {
		assert(j.setIndex <= disabledSet)
	}

	// Remove joint from solver set that owns it
	setIndex := j.setIndex
	localIndex := j.localIndex

	if setIndex == awakeSet {
		w.removeJointFromGraph(j.edges[0].bodyID, j.edges[1].bodyID, j.colorIndex, localIndex)
	} else {
		set := &w.solverSets[setIndex]
		movedIndex := removeSwap(&set.jointSims, localIndex)
		if movedIndex != NullIndex {
			// Fix moved joint
			movedJointSim := &set.jointSims[localIndex]
			movedID := movedJointSim.jointID
			movedJoint := &w.joints[movedID]
			assert(movedJoint.localIndex == movedIndex)
			movedJoint.localIndex = localIndex
		}
	}

	// Free joint and id (preserve joint generation)
	j.setIndex = NullIndex
	j.localIndex = NullIndex
	j.colorIndex = NullIndex
	j.jointID = NullIndex
	freeID(&w.jointIDPool, jointID)

	if wakeBodies {
		w.wakeBody(bodyA)
		w.wakeBody(bodyB)
	}

	w.validateSolverSets()
}

// DestroyJoint destroys a joint. wakeAttached wakes the attached bodies
// (upstream b2DestroyJoint).
func (w *World) DestroyJoint(jointID JointID, wakeAttached bool) {
	assert(!w.locked)
	if w.locked {
		return
	}

	// Reject an id minted by another world (see DestroyBody).
	if !w.ownsToken(jointID.world0) {
		assert(false)
		return
	}

	j := w.getJointFullID(jointID)

	w.destroyJointInternal(j, wakeAttached)
}

// ---------------------------------------------------------------------------
// Public accessors
// ---------------------------------------------------------------------------

// JointType returns the joint type (upstream b2Joint_GetType).
func (w *World) JointType(jointID JointID) JointType {
	j := w.getJointFullID(jointID)
	return j.jointType
}

// JointBodyA returns body A id on a joint (upstream b2Joint_GetBodyA).
func (w *World) JointBodyA(jointID JointID) BodyID {
	j := w.getJointFullID(jointID)
	return w.makeBodyID(j.edges[0].bodyID)
}

// JointBodyB returns body B id on a joint (upstream b2Joint_GetBodyB).
func (w *World) JointBodyB(jointID JointID) BodyID {
	j := w.getJointFullID(jointID)
	return w.makeBodyID(j.edges[1].bodyID)
}

// SetJointLocalFrameA sets the local frame on bodyA
// (upstream b2Joint_SetLocalFrameA).
func (w *World) SetJointLocalFrameA(jointID JointID, localFrame Transform) {
	assert(IsValidTransform(localFrame))

	j := w.getJointFullID(jointID)
	js := w.getJointSim(j)
	js.localFrameA = localFrame
}

// JointLocalFrameA returns the local frame on bodyA
// (upstream b2Joint_GetLocalFrameA).
func (w *World) JointLocalFrameA(jointID JointID) Transform {
	j := w.getJointFullID(jointID)
	js := w.getJointSim(j)
	return js.localFrameA
}

// SetJointLocalFrameB sets the local frame on bodyB
// (upstream b2Joint_SetLocalFrameB).
func (w *World) SetJointLocalFrameB(jointID JointID, localFrame Transform) {
	assert(IsValidTransform(localFrame))

	j := w.getJointFullID(jointID)
	js := w.getJointSim(j)
	js.localFrameB = localFrame
}

// JointLocalFrameB returns the local frame on bodyB
// (upstream b2Joint_GetLocalFrameB).
func (w *World) JointLocalFrameB(jointID JointID) Transform {
	j := w.getJointFullID(jointID)
	js := w.getJointSim(j)
	return js.localFrameB
}

// SetJointCollideConnected toggles collision between connected bodies
// (upstream b2Joint_SetCollideConnected).
//
// Note: enabling collision may add contacts on the next broad-phase update;
// disabling collision destroys the existing contacts between the bodies.
func (w *World) SetJointCollideConnected(jointID JointID, shouldCollide bool) {
	assert(!w.locked)
	if w.locked {
		return
	}

	j := w.getJointFullID(jointID)
	if j.collideConnected == shouldCollide {
		return
	}

	j.collideConnected = shouldCollide

	bodyA := &w.bodies[j.edges[0].bodyID]
	bodyB := &w.bodies[j.edges[1].bodyID]

	if shouldCollide {
		// need to tell the broad-phase to look for new pairs for one of the
		// two bodies. Pick the one with the fewest shapes.
		shapeCountA := bodyA.shapeCount
		shapeCountB := bodyB.shapeCount

		shapeID := bodyB.headShapeID
		if shapeCountA < shapeCountB {
			shapeID = bodyA.headShapeID
		}
		for shapeID != NullIndex {
			s := &w.shapes[shapeID]

			if s.proxyKey != NullIndex {
				w.broadPhase.bufferMove(s.proxyKey)
			}

			shapeID = s.nextShapeID
		}
	} else {
		w.destroyContactsBetweenBodies(bodyA, bodyB)
	}
}

// JointCollideConnected reports whether the connected bodies may collide
// (upstream b2Joint_GetCollideConnected).
func (w *World) JointCollideConnected(jointID JointID) bool {
	j := w.getJointFullID(jointID)
	return j.collideConnected
}

// SetJointUserData sets the user data on a joint
// (upstream b2Joint_SetUserData).
func (w *World) SetJointUserData(jointID JointID, userData uint64) {
	j := w.getJointFullID(jointID)
	j.userData = userData
}

// JointUserData returns the user data on a joint
// (upstream b2Joint_GetUserData).
func (w *World) JointUserData(jointID JointID) uint64 {
	j := w.getJointFullID(jointID)
	return j.userData
}

// WakeJointBodies wakes the bodies connected to this joint
// (upstream b2Joint_WakeBodies).
func (w *World) WakeJointBodies(jointID JointID) {
	assert(!w.locked)
	if w.locked {
		return
	}

	j := w.getJointFullID(jointID)
	bodyA := &w.bodies[j.edges[0].bodyID]
	bodyB := &w.bodies[j.edges[1].bodyID]

	w.wakeBody(bodyA)
	w.wakeBody(bodyB)
}

// getJointReaction returns the total force and torque generated by a joint
// this step, used for the joint event thresholds
// (upstream b2GetJointReaction).
func getJointReaction(sim *jointSim, invTimeStep float64) (float64, float64) {
	linearImpulse := 0.0
	angularImpulse := 0.0

	switch sim.jointType {
	case DistanceJoint:
		j := &sim.distanceJoint
		linearImpulse = absFloat(j.impulse + j.lowerImpulse - j.upperImpulse + j.motorImpulse)

	case MotorJoint:
		j := &sim.motorJoint
		linearImpulse = Length(Add(j.linearVelocityImpulse, j.linearSpringImpulse))
		angularImpulse = absFloat(j.angularVelocityImpulse + j.angularSpringImpulse)

	case FilterJoint:
		// no reaction

	case PrismaticJoint:
		j := &sim.prismaticJoint
		perpImpulse := j.impulse.X
		axialImpulse := j.motorImpulse + j.lowerImpulse - j.upperImpulse
		// linearImpulse = sqrt(perpImpulse^2 + axialImpulse^2)
		linearImpulse = math.Sqrt(float64(perpImpulse*perpImpulse) + float64(axialImpulse*axialImpulse))
		angularImpulse = absFloat(j.impulse.Y)

	case RevoluteJoint:
		j := &sim.revoluteJoint

		linearImpulse = Length(j.linearImpulse)
		angularImpulse = absFloat(j.motorImpulse + j.lowerImpulse - j.upperImpulse)

	case WeldJoint:
		j := &sim.weldJoint
		linearImpulse = Length(j.linearImpulse)
		angularImpulse = absFloat(j.angularImpulse)

	case WheelJoint:
		j := &sim.wheelJoint
		perpImpulse := j.perpImpulse
		axialImpulse := j.springImpulse + j.lowerImpulse - j.upperImpulse
		// linearImpulse = sqrt(perpImpulse^2 + axialImpulse^2)
		linearImpulse = math.Sqrt(float64(perpImpulse*perpImpulse) + float64(axialImpulse*axialImpulse))
		angularImpulse = absFloat(j.motorImpulse)

	default:
	}

	return linearImpulse * invTimeStep, angularImpulse * invTimeStep
}

// getJointConstraintForce dispatches the constraint force by joint type
// (upstream static b2GetJointConstraintForce).
func (w *World) getJointConstraintForce(j *joint) Vec2 {
	base := w.getJointSim(j)

	switch j.jointType {
	case DistanceJoint:
		return getDistanceJointForce(w, base)

	case MotorJoint:
		return getMotorJointForce(w, base)

	case FilterJoint:
		return Vec2Zero

	case PrismaticJoint:
		return getPrismaticJointForce(w, base)

	case RevoluteJoint:
		return getRevoluteJointForce(w, base)

	case WeldJoint:
		return getWeldJointForce(w, base)

	case WheelJoint:
		return getWheelJointForce(w, base)

	default:
		assert(false)
		return Vec2Zero
	}
}

// getJointConstraintTorque dispatches the constraint torque by joint type
// (upstream static b2GetJointConstraintTorque).
func (w *World) getJointConstraintTorque(j *joint) float64 {
	base := w.getJointSim(j)

	switch j.jointType {
	case DistanceJoint:
		return 0.0

	case MotorJoint:
		return getMotorJointTorque(w, base)

	case FilterJoint:
		return 0.0

	case PrismaticJoint:
		return getPrismaticJointTorque(w, base)

	case RevoluteJoint:
		return getRevoluteJointTorque(w, base)

	case WeldJoint:
		return getWeldJointTorque(w, base)

	case WheelJoint:
		return getWheelJointTorque(w, base)

	default:
		assert(false)
		return 0.0
	}
}

// drawJoint renders a single joint and its optional graph-color and force/torque
// extras (upstream b2DrawJoint).
func (w *World) drawJoint(draw *DebugDraw, j *joint) {
	bodyA := &w.bodies[j.edges[0].bodyID]
	bodyB := &w.bodies[j.edges[1].bodyID]

	if bodyA.setIndex == disabledSet || bodyB.setIndex == disabledSet {
		return
	}

	jointSim := w.getJointSim(j)

	transformA := w.getBodyTransformQuick(bodyA)
	transformB := w.getBodyTransformQuick(bodyB)
	pA := TransformPoint(transformA, jointSim.localFrameA.P)
	pB := TransformPoint(transformB, jointSim.localFrameB.P)

	scale := maxFloat(0.0001, draw.JointScale*j.drawScale)

	switch j.jointType {
	case DistanceJoint:
		drawDistanceJoint(draw, jointSim, transformA, transformB)

	case FilterJoint:
		draw.DrawLineFcn(pA, pB, ColorGold, draw.Context)

	case MotorJoint:
		draw.DrawPointFcn(pA, 8.0, ColorYellowGreen, draw.Context)
		draw.DrawPointFcn(pB, 8.0, ColorPlum, draw.Context)
		draw.DrawLineFcn(pA, pB, ColorLightGray, draw.Context)

	case PrismaticJoint:
		drawPrismaticJoint(draw, jointSim, transformA, transformB, scale)

	case RevoluteJoint:
		drawRevoluteJoint(draw, jointSim, transformA, transformB, scale)

	case WeldJoint:
		drawWeldJoint(draw, jointSim, transformA, transformB, scale)

	case WheelJoint:
		drawWheelJoint(draw, jointSim, transformA, transformB, scale)

	default:
		color := ColorDarkSeaGreen
		draw.DrawLineFcn(transformA.P, pA, color, draw.Context)
		draw.DrawLineFcn(pA, pB, color, draw.Context)
		draw.DrawLineFcn(transformB.P, pB, color, draw.Context)
	}

	if draw.DrawGraphColors {
		colorIndex := j.colorIndex
		if colorIndex != NullIndex {
			p := Lerp(pA, pB, 0.5)
			draw.DrawPointFcn(p, 5.0, graphColorPalette[colorIndex], draw.Context)
		}
	}

	if draw.DrawJointExtras {
		force := w.getJointConstraintForce(j)
		torque := w.getJointConstraintTorque(j)
		p := Lerp(pA, pB, 0.5)

		draw.DrawLineFcn(p, MulAdd(p, 0.001, force), ColorAzure, draw.Context)
		draw.DrawStringFcn(p, "f = ["+strconv.FormatFloat(force.X, 'g', -1, 64)+", "+strconv.FormatFloat(force.Y, 'g', -1, 64)+"], t = "+strconv.FormatFloat(torque, 'g', -1, 64), ColorAzure, draw.Context)
	}
}

// drawWeldJoint renders a weld joint (upstream b2DrawWeldJoint).
func drawWeldJoint(draw *DebugDraw, base *jointSim, transformA, transformB Transform, drawScale float64) {
	assert(base.jointType == WeldJoint)

	frameA := MulTransforms(transformA, base.localFrameA)
	frameB := MulTransforms(transformB, base.localFrameB)

	box := MakeBox(0.25*drawScale, 0.125*drawScale)

	var points [4]Vec2
	for i := range 4 {
		//nolint:gosec // G602: the loop bound is the literal 4 and points is [4]Vec2; box comes from MakeBox, whose Vertices is [MaxPolygonVertices]Vec2 with Count 4.
		points[i] = TransformPoint(frameA, box.Vertices[i])
	}
	draw.DrawPolygonFcn(points[:4], ColorDarkOrange, draw.Context)

	for i := range 4 {
		//nolint:gosec // G602: the loop bound is the literal 4 and points is [4]Vec2; box comes from MakeBox, whose Vertices is [MaxPolygonVertices]Vec2 with Count 4.
		points[i] = TransformPoint(frameB, box.Vertices[i])
	}
	draw.DrawPolygonFcn(points[:4], ColorDarkCyan, draw.Context)
}

// JointConstraintForce returns the current constraint force for this joint.
// Usually in Newtons (upstream b2Joint_GetConstraintForce).
func (w *World) JointConstraintForce(jointID JointID) Vec2 {
	j := w.getJointFullID(jointID)
	return w.getJointConstraintForce(j)
}

// JointConstraintTorque returns the current constraint torque for this joint.
// Usually in Newton * meters (upstream b2Joint_GetConstraintTorque).
func (w *World) JointConstraintTorque(jointID JointID) float64 {
	j := w.getJointFullID(jointID)
	return w.getJointConstraintTorque(j)
}

// JointLinearSeparation returns the current linear separation error for this
// joint. Does not consider admissible movement such as the target length of a
// spring (upstream b2Joint_GetLinearSeparation).
func (w *World) JointLinearSeparation(jointID JointID) float64 {
	j := w.getJointFullID(jointID)
	base := w.getJointSim(j)

	xfA := w.getBodyTransform(j.edges[0].bodyID)
	xfB := w.getBodyTransform(j.edges[1].bodyID)

	pA := TransformPoint(xfA, base.localFrameA.P)
	pB := TransformPoint(xfB, base.localFrameB.P)
	dp := Sub(pB, pA)

	switch j.jointType {
	case DistanceJoint:
		dj := &base.distanceJoint
		length := Length(dp)
		if dj.enableSpring {
			if dj.enableLimit {
				if length < dj.minLength {
					return dj.minLength - length
				}

				if length > dj.maxLength {
					return length - dj.maxLength
				}

				return 0.0
			}

			return 0.0
		}

		return absFloat(length - dj.length)

	case MotorJoint:
		return 0.0

	case FilterJoint:
		return 0.0

	case PrismaticJoint:
		pj := &base.prismaticJoint
		axisA := RotateVector(xfA.Q, Vec2{X: 1.0, Y: 0.0})
		perpA := LeftPerp(axisA)
		perpendicularSeparation := absFloat(Dot(perpA, dp))
		limitSeparation := 0.0

		if pj.enableLimit {
			translation := Dot(axisA, dp)
			if translation < pj.lowerTranslation {
				limitSeparation = pj.lowerTranslation - translation
			}

			if pj.upperTranslation < translation {
				limitSeparation = translation - pj.upperTranslation
			}
		}

		// sqrt(perpendicularSeparation^2 + limitSeparation^2)
		return math.Sqrt(float64(perpendicularSeparation*perpendicularSeparation) + float64(limitSeparation*limitSeparation))

	case RevoluteJoint:
		return Length(dp)

	case WeldJoint:
		wj := &base.weldJoint
		if wj.linearHertz == 0.0 {
			return Length(dp)
		}

		return 0.0

	case WheelJoint:
		wj := &base.wheelJoint
		axisA := RotateVector(xfA.Q, Vec2{X: 1.0, Y: 0.0})
		perpA := LeftPerp(axisA)
		perpendicularSeparation := absFloat(Dot(perpA, dp))
		limitSeparation := 0.0

		if wj.enableLimit {
			translation := Dot(axisA, dp)
			if translation < wj.lowerTranslation {
				limitSeparation = wj.lowerTranslation - translation
			}

			if wj.upperTranslation < translation {
				limitSeparation = translation - wj.upperTranslation
			}
		}

		// sqrt(perpendicularSeparation^2 + limitSeparation^2)
		return math.Sqrt(float64(perpendicularSeparation*perpendicularSeparation) + float64(limitSeparation*limitSeparation))

	default:
		assert(false)
		return 0.0
	}
}

// JointAngularSeparation returns the current angular separation error for
// this joint. Does not consider admissible movement such as the target angle
// of a spring (upstream b2Joint_GetAngularSeparation).
func (w *World) JointAngularSeparation(jointID JointID) float64 {
	j := w.getJointFullID(jointID)
	base := w.getJointSim(j)

	xfA := w.getBodyTransform(j.edges[0].bodyID)
	xfB := w.getBodyTransform(j.edges[1].bodyID)
	relativeAngle := RelativeAngle(xfA.Q, xfB.Q)

	switch j.jointType {
	case DistanceJoint:
		return 0.0

	case MotorJoint:
		return 0.0

	case FilterJoint:
		return 0.0

	case PrismaticJoint:
		return relativeAngle

	case RevoluteJoint:
		rj := &base.revoluteJoint
		if rj.enableLimit {
			angle := relativeAngle
			if angle < rj.lowerAngle {
				return rj.lowerAngle - angle
			}

			if rj.upperAngle < angle {
				return angle - rj.upperAngle
			}
		}

		return 0.0

	case WeldJoint:
		wj := &base.weldJoint
		if wj.angularHertz == 0.0 {
			return relativeAngle
		}

		return 0.0

	case WheelJoint:
		return 0.0

	default:
		assert(false)
		return 0.0
	}
}

// SetJointConstraintTuning sets the joint constraint tuning. Advanced
// feature. hertz is the stiffness in cycles per second (use zero for the most
// rigid behavior), dampingRatio the non-dimensional damping (one is critical
// damping) (upstream b2Joint_SetConstraintTuning).
func (w *World) SetJointConstraintTuning(jointID JointID, hertz, dampingRatio float64) {
	assert(IsValidFloat(hertz) && hertz >= 0.0)
	assert(IsValidFloat(dampingRatio) && dampingRatio >= 0.0)

	j := w.getJointFullID(jointID)
	base := w.getJointSim(j)
	base.constraintHertz = hertz
	base.constraintDampingRatio = dampingRatio
}

// JointConstraintTuning returns the joint constraint tuning as
// (hertz, dampingRatio) (upstream b2Joint_GetConstraintTuning).
func (w *World) JointConstraintTuning(jointID JointID) (float64, float64) {
	j := w.getJointFullID(jointID)
	base := w.getJointSim(j)
	return base.constraintHertz, base.constraintDampingRatio
}

// SetJointForceThreshold sets the force threshold for joint events, usually
// in Newtons (upstream b2Joint_SetForceThreshold).
func (w *World) SetJointForceThreshold(jointID JointID, threshold float64) {
	assert(IsValidFloat(threshold) && threshold >= 0.0)

	j := w.getJointFullID(jointID)
	base := w.getJointSim(j)
	base.forceThreshold = threshold
}

// JointForceThreshold returns the force threshold for joint events
// (upstream b2Joint_GetForceThreshold).
func (w *World) JointForceThreshold(jointID JointID) float64 {
	j := w.getJointFullID(jointID)
	base := w.getJointSim(j)
	return base.forceThreshold
}

// SetJointTorqueThreshold sets the torque threshold for joint events, usually
// in Newton * meters (upstream b2Joint_SetTorqueThreshold).
func (w *World) SetJointTorqueThreshold(jointID JointID, threshold float64) {
	assert(IsValidFloat(threshold) && threshold >= 0.0)

	j := w.getJointFullID(jointID)
	base := w.getJointSim(j)
	base.torqueThreshold = threshold
}

// JointTorqueThreshold returns the torque threshold for joint events
// (upstream b2Joint_GetTorqueThreshold).
func (w *World) JointTorqueThreshold(jointID JointID) float64 {
	j := w.getJointFullID(jointID)
	base := w.getJointSim(j)
	return base.torqueThreshold
}

// ---------------------------------------------------------------------------
// Solver dispatch
// ---------------------------------------------------------------------------

// prepareJoint prepares a joint for solving (upstream b2PrepareJoint).
func (w *World) prepareJoint(js *jointSim, ctx *stepContext) {
	// Clamp joint hertz based on the time step to reduce jitter.
	hertz := minFloat(js.constraintHertz, 0.25*ctx.invH)
	js.constraintSoftness = makeSoft(hertz, js.constraintDampingRatio, ctx.h)

	switch js.jointType {
	case DistanceJoint:
		prepareDistanceJoint(js, ctx)

	case MotorJoint:
		prepareMotorJoint(js, ctx)

	case FilterJoint:
		// no solve work

	case PrismaticJoint:
		preparePrismaticJoint(js, ctx)

	case RevoluteJoint:
		prepareRevoluteJoint(js, ctx)

	case WeldJoint:
		prepareWeldJoint(js, ctx)

	case WheelJoint:
		prepareWheelJoint(js, ctx)

	default:
		assert(false)
	}
}

// warmStartJoint warm starts a joint (upstream b2WarmStartJoint).
func (w *World) warmStartJoint(js *jointSim, ctx *stepContext) {
	switch js.jointType {
	case DistanceJoint:
		warmStartDistanceJoint(js, ctx)

	case MotorJoint:
		warmStartMotorJoint(js, ctx)

	case FilterJoint:
		// no solve work

	case PrismaticJoint:
		warmStartPrismaticJoint(js, ctx)

	case RevoluteJoint:
		warmStartRevoluteJoint(js, ctx)

	case WeldJoint:
		warmStartWeldJoint(js, ctx)

	case WheelJoint:
		warmStartWheelJoint(js, ctx)

	default:
		assert(false)
	}
}

// solveJoint solves a joint's velocity constraints (upstream b2SolveJoint).
func (w *World) solveJoint(js *jointSim, ctx *stepContext, useBias bool) {
	switch js.jointType {
	case DistanceJoint:
		solveDistanceJoint(js, ctx, useBias)

	case MotorJoint:
		solveMotorJoint(js, ctx)

	case FilterJoint:
		// no solve work

	case PrismaticJoint:
		solvePrismaticJoint(js, ctx, useBias)

	case RevoluteJoint:
		solveRevoluteJoint(js, ctx, useBias)

	case WeldJoint:
		solveWeldJoint(js, ctx, useBias)

	case WheelJoint:
		solveWheelJoint(js, ctx, useBias)

	default:
		assert(false)
	}
}
