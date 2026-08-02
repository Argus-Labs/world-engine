// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/solver.h, src/solver.c
// (the continuous-collision functions live in solver_continuous.go).
//
// SINGLE-THREADED STAGE EXECUTION (approved deviation): upstream builds
// b2SolverStage/b2SolverBlock arrays and drives them from b2SolverTask with
// atomic work stealing across workers. This port executes the exact same
// stage sequence as serial loops. Mapping (upstream b2SolverStageType and the
// b2SolverTask main-thread flow → this port):
//
//	b2_stagePrepareJoints       → prepareJoint loop over active colors, then
//	                              overflow (b2PrepareOverflowJoints)
//	b2_stagePrepareContacts     → prepareContactsColor over active colors,
//	                              then overflow (b2PrepareOverflowContacts)
//	per sub-step:
//	b2_stageIntegrateVelocities → integrateVelocitiesTask (all awake bodies)
//	b2_stageWarmStart           → overflow joints, overflow contacts, then per
//	                              active color: joints then contacts
//	b2_stageSolve               → useBias=true: overflow joints, overflow
//	                              contacts, then per color: joints, contacts
//	b2_stageIntegratePositions  → integratePositionsTask (all awake bodies)
//	b2_stageRelax               → useBias=false: overflow joints, overflow
//	                              contacts, then per color: joints, contacts
//	end of sub-steps:
//	b2_stageRestitution         → overflow restitution, then active colors
//	b2_stageStoreImpulses       → overflow store, then active colors
//
// Within every stage the overflow color runs before the active colors (the
// upstream main thread runs the b2*Overflow* calls before executing the
// per-color stages) and joint blocks run before contact blocks within a color
// (upstream block order). Active colors run in ascending color-index order.
// ITERATIONS and RELAX_ITERATIONS are both 1 upstream; the loops are kept.
//
// Continuous collision (b2SolveContinuous, b2ContinuousQueryCallback) lives
// in solver_continuous.go. The upstream b2BulletBodyTask is the serial bullet
// loop in the solve epilogue below; finalizeBodiesTask fills the bulletBodies
// array in ascending body-sim index order, which keeps the serial bullet
// processing deterministic.

package box2d

import "math"

// maxJointEventThreshold is the upstream FLT_MAX sentinel used by the joint
// event scan: joints with both thresholds at or above this value never
// generate joint events (see b2SolveJointsTask in solver.c).
const maxJointEventThreshold = math.MaxFloat32

// softness is the soft-constraint coefficient bundle (upstream b2Softness).
type softness struct {
	biasRate     float64
	massScale    float64
	impulseScale float64
}

// makeSoft computes soft constraint coefficients from frequency, damping
// ratio and time step (upstream b2MakeSoft in solver.h). Purely algebraic —
// no transcendentals.
func makeSoft(hertz, zeta, h float64) softness {
	if hertz == 0.0 {
		return softness{biasRate: 0.0, massScale: 0.0, impulseScale: 0.0}
	}

	// bias = w / (2 * z + hw)
	// massScale = hw * (2 * z + hw) / (1 + hw * (2 * z + hw))
	// impulseScale = 1 / (1 + hw * (2 * z + hw))
	//
	// If z == 0
	// bias = 1/h
	// massScale = hw^2 / (1 + hw^2)
	// impulseScale = 1 / (1 + hw^2)
	//
	// In all cases:
	// massScale + impulseScale == 1
	omega := 2.0 * Pi * hertz
	// a1 = 2.0 * zeta + h * omega
	a1 := float64(2.0*zeta) + float64(h*omega)
	// a2 = h * omega * a1
	a2 := float64(float64(h*omega) * a1)
	a3 := 1.0 / (1.0 + a2)

	return softness{
		biasRate:     omega / a1,
		massScale:    a2 * a3,
		impulseScale: a3,
	}
}

// stepContext is the context for a time step, recreated each step (upstream
// b2StepContext).
//
// Deviations from upstream (single-threaded port): the task/atomic machinery
// (stages, blocks, atomicSyncBits, workerCount) is gone — see the file
// header. The flattened joint/contact pointer arrays and the SIMD-wide
// constraint buffer are not needed: the solver iterates the graph colors
// directly and the scalar constraints live per color (graphColor.constraints).
type stepContext struct {
	// time step
	dt float64

	// inverse time step (0 if dt == 0)
	invDT float64

	// sub-step
	h    float64
	invH float64

	subStepCount int

	contactSoftness softness
	staticSoftness  softness

	restitutionThreshold float64
	maxLinearVelocity    float64

	world *World
	graph *constraintGraph

	// shortcut to body states from awake set
	states []bodyState

	// shortcut to body sims from awake set
	sims []bodySim

	// contact pointers gathered for the narrow phase (upstream
	// b2StepContext.contacts): graph colors in ascending order, then the
	// awake non-touching contacts. Only valid during collide.
	contacts []*contactSim

	// Array of bullet bodies that need continuous collision handling.
	bulletBodies    []int
	bulletBodyCount int

	activeColorCount int

	enableWarmStarting bool
}

// taskContext is the single-threaded counterpart of the per-worker upstream
// b2TaskContext (physics_world.h). The bit sets are sized/cleared each step.
type taskContext struct {
	// Used to track a contact state change that affects islands or events
	// (bit index = contact id).
	contactStateBitSet bitSet

	// Used to sort shape AABB enlargement in deterministic body-sim order
	// (bit index = awake body sim index).
	enlargedSimBitSet bitSet

	// Used to track islands that stay awake (bit index = island local index).
	awakeIslandBitSet bitSet

	// Used to report joints with force/torque events (bit index = joint id).
	// Cleared before the solve; set by solveJointsColor.
	jointStateBitSet bitSet

	// Shapes that hit sensors during a continuous sweep, collected by
	// solveContinuous, reported into the per-sensor hit arrays after the
	// bullet stage, and cleared before body finalization each step.
	sensorHits []sensorHit

	// Per-step sleepiest split island candidate.
	splitIslandID  int
	splitSleepTime float64
}

func createTaskContext() taskContext {
	return taskContext{
		contactStateBitSet: createBitSet(1024),
		enlargedSimBitSet:  createBitSet(256),
		awakeIslandBitSet:  createBitSet(256),
		jointStateBitSet:   createBitSet(1024),
		splitIslandID:      NullIndex,
	}
}

func destroyTaskContext(tc *taskContext) {
	destroyBitSet(&tc.contactStateBitSet)
	destroyBitSet(&tc.enlargedSimBitSet)
	destroyBitSet(&tc.awakeIslandBitSet)
	destroyBitSet(&tc.jointStateBitSet)
	*tc = taskContext{}
}

// solveJointsColor solves the joints of one active graph color and performs
// the joint event bookkeeping (upstream b2SolveJointsTask). Note that
// upstream overflow joints run through b2SolveOverflowJoints, which has no
// event bookkeeping — the overflow loops in solve call solveJoint directly to
// match.
func (w *World) solveJointsColor(ctx *stepContext, colorIndex int, useBias bool) {
	color := &ctx.graph.colors[colorIndex]
	jointStateBitSet := &w.taskContext.jointStateBitSet

	for i := range color.jointSims {
		j := &color.jointSims[i]
		w.solveJoint(j, ctx, useBias)

		if useBias && (j.forceThreshold < maxJointEventThreshold || j.torqueThreshold < maxJointEventThreshold) &&
			!getBit(jointStateBitSet, uint32(j.jointID)) {
			force, torque := getJointReaction(j, ctx.invH)

			// Check thresholds. A zero threshold means all awake joints get
			// reported.
			if force >= j.forceThreshold || torque >= j.torqueThreshold {
				// Flag this joint for processing.
				setBit(jointStateBitSet, uint32(j.jointID))
			}
		}
	}
}

// integrateVelocitiesTask integrates velocities and applies damping, gravity
// and speed clamps (upstream b2IntegrateVelocitiesTask).
func integrateVelocitiesTask(startIndex, endIndex int, ctx *stepContext) {
	states := ctx.states
	sims := ctx.sims

	gravity := ctx.world.gravity
	h := ctx.h
	maxLinearSpeed := ctx.maxLinearVelocity
	maxAngularSpeed := MaxRotation * ctx.invDT
	maxLinearSpeedSquared := maxLinearSpeed * maxLinearSpeed
	maxAngularSpeedSquared := maxAngularSpeed * maxAngularSpeed

	for i := startIndex; i < endIndex; i++ {
		sim := &sims[i]
		state := &states[i]

		v := state.linearVelocity
		wAng := state.angularVelocity

		// Apply forces, torque, gravity, and damping
		// Apply damping.
		// Differential equation: dv/dt + c * v = 0
		// Solution: v(t) = v0 * exp(-c * t)
		// Time step: v(t + dt) = v0 * exp(-c * (t + dt)) = v0 * exp(-c * t) * exp(-c * dt) = v(t) * exp(-c * dt)
		// v2 = exp(-c * dt) * v1
		// Pade approximation:
		// v2 = v1 * 1 / (1 + c * dt)
		linearDamping := 1.0 / (1.0 + float64(h*sim.linearDamping))
		angularDamping := 1.0 / (1.0 + float64(h*sim.angularDamping))

		// Gravity scale will be zero for kinematic bodies
		gravityScale := 0.0
		if sim.invMass > 0.0 {
			gravityScale = sim.gravityScale
		}

		// lvd = h * im * f + h * g
		linearVelocityDelta := Add(MulSV(h*sim.invMass, sim.force), MulSV(h*gravityScale, gravity))
		angularVelocityDelta := float64(h * sim.invInertia * sim.torque)

		v = MulAdd(linearVelocityDelta, linearDamping, v)
		// w = angularVelocityDelta + angularDamping * w
		wAng = angularVelocityDelta + float64(angularDamping*wAng)

		// Clamp to max linear speed
		if Dot(v, v) > maxLinearSpeedSquared {
			ratio := maxLinearSpeed / Length(v)
			v = MulSV(ratio, v)
			sim.flags |= isSpeedCapped
		}

		// Clamp to max angular speed
		if wAng*wAng > maxAngularSpeedSquared && sim.flags&allowFastRotation == 0 {
			ratio := maxAngularSpeed / absFloat(wAng)
			wAng *= ratio
			sim.flags |= isSpeedCapped
		}

		if state.flags&lockLinearX != 0 {
			v.X = 0.0
		}

		if state.flags&lockLinearY != 0 {
			v.Y = 0.0
		}

		if state.flags&lockAngularZ != 0 {
			wAng = 0.0
		}

		state.linearVelocity = v
		state.angularVelocity = wAng
	}
}

// integratePositionsTask integrates the delta position and rotation of the
// body states (upstream b2IntegratePositionsTask).
func integratePositionsTask(startIndex, endIndex int, ctx *stepContext) {
	states := ctx.states
	h := ctx.h

	assert(startIndex <= endIndex)

	for i := startIndex; i < endIndex; i++ {
		state := &states[i]

		if state.flags&lockLinearX != 0 {
			state.linearVelocity.X = 0.0
		}

		if state.flags&lockLinearY != 0 {
			state.linearVelocity.Y = 0.0
		}

		if state.flags&lockAngularZ != 0 {
			state.angularVelocity = 0.0
		}

		state.deltaPosition = MulAdd(state.deltaPosition, h, state.linearVelocity)
		state.deltaRotation = IntegrateRotation(state.deltaRotation, h*state.angularVelocity)
	}
}

// finalizeBodiesTask finalizes body transforms after the solve: applies the
// position/rotation deltas, emits body move events, accumulates sleep time,
// runs the continuous solve for fast non-bullet bodies, gathers fast bullet
// bodies for the bullet stage and refreshes shape AABBs with broad-phase
// enlarge buffering in deterministic body order (upstream
// b2FinalizeBodiesTask).
func (w *World) finalizeBodiesTask(startIndex, endIndex int, ctx *stepContext) {
	enableSleep := w.enableSleep
	states := ctx.states
	sims := ctx.sims
	bodies := w.bodies
	timeStep := ctx.dt
	invTimeStep := ctx.invDT

	worldID := w.worldID

	// The body move event array should already have the correct size.
	assert(endIndex <= len(w.bodyMoveEvents))
	moveEvents := w.bodyMoveEvents

	tc := &w.taskContext
	enlargedSimBitSet := &tc.enlargedSimBitSet
	awakeIslandBitSet := &tc.awakeIslandBitSet

	enableContinuous := w.enableContinuous

	const speculativeDistance = SpeculativeDistance

	assert(startIndex <= endIndex)

	for simIndex := startIndex; simIndex < endIndex; simIndex++ {
		state := &states[simIndex]
		sim := &sims[simIndex]

		if state.flags&lockLinearX != 0 {
			state.linearVelocity.X = 0.0
		}

		if state.flags&lockLinearY != 0 {
			state.linearVelocity.Y = 0.0
		}

		if state.flags&lockAngularZ != 0 {
			state.angularVelocity = 0.0
		}

		v := state.linearVelocity
		wAng := state.angularVelocity

		assert(IsValidVec2(v))
		assert(IsValidFloat(wAng))

		sim.center = Add(sim.center, state.deltaPosition)
		sim.transform.Q = NormalizeRot(MulRot(state.deltaRotation, sim.transform.Q))

		// Use the velocity of the farthest point on the body to account for
		// rotation.
		// maxVelocity = length(v) + abs(w) * maxExtent
		maxVelocity := Length(v) + float64(absFloat(wAng)*sim.maxExtent)

		// Sleep needs to observe position correction as well as true velocity.
		// maxDeltaPosition = length(deltaPosition) + abs(deltaRotation.s) * maxExtent
		maxDeltaPosition := Length(state.deltaPosition) + float64(absFloat(state.deltaRotation.S)*sim.maxExtent)

		// Position correction is not as important for sleep as true velocity.
		positionSleepFactor := 0.5

		sleepVelocity := maxFloat(maxVelocity, positionSleepFactor*invTimeStep*maxDeltaPosition)

		// reset state deltas
		state.deltaPosition = Vec2Zero
		state.deltaRotation = RotIdentity

		sim.transform.P = Sub(sim.center, RotateVector(sim.transform.Q, sim.localCenter))

		// cache miss here, however I need the shape list below
		b := &bodies[sim.bodyID]
		b.bodyMoveIndex = simIndex
		moveEvents[simIndex].Transform = sim.transform
		moveEvents[simIndex].BodyID = BodyID{index1: int32(sim.bodyID + 1), world0: worldID, generation: b.generation}
		moveEvents[simIndex].UserData = b.userData
		moveEvents[simIndex].FellAsleep = false

		// reset applied force and torque
		sim.force = Vec2Zero
		sim.torque = 0.0

		// If you hit this then it means you deferred mass computation but
		// never called ApplyBodyMassFromShapes.
		assert(b.flags&dirtyMass == 0)

		b.flags &^= isFast | isSpeedCapped | hadTimeOfImpact
		b.flags |= sim.flags & (isSpeedCapped | hadTimeOfImpact)
		sim.flags &^= isFast | isSpeedCapped | hadTimeOfImpact

		if !enableSleep || !b.enableSleep || sleepVelocity > b.sleepThreshold {
			// Body is not sleepy
			b.sleepTime = 0.0

			if b.bodyType == DynamicBody && enableContinuous && maxVelocity*timeStep > 0.5*sim.minExtent {
				// This flag is only retained for debug draw
				sim.flags |= isFast

				// Store in fast array for the continuous collision stage.
				// This is deterministic because the order of TOI sweeps
				// doesn't matter.
				if sim.flags&isBullet != 0 {
					ctx.bulletBodies[ctx.bulletBodyCount] = simIndex
					ctx.bulletBodyCount++
				} else {
					w.solveContinuous(simIndex)
				}
			} else {
				// Body is safe to advance
				sim.center0 = sim.center
				sim.rotation0 = sim.transform.Q
			}
		} else {
			// Body is safe to advance and is falling asleep
			sim.center0 = sim.center
			sim.rotation0 = sim.transform.Q
			b.sleepTime += timeStep
		}

		// Any single body in an island can keep it awake
		isl := &w.islands[b.islandID]
		if b.sleepTime < TimeToSleep {
			// keep island awake
			islandIndex := isl.localIndex
			setBit(awakeIslandBitSet, uint32(islandIndex))
		} else if isl.constraintRemoveCount > 0 {
			// body wants to sleep but its island needs splitting first
			if b.sleepTime > tc.splitSleepTime {
				// pick the sleepiest candidate
				tc.splitIslandID = b.islandID
				tc.splitSleepTime = b.sleepTime
			}
		}

		// Update shapes AABBs
		transform := sim.transform
		isFastBody := sim.flags&isFast != 0
		shapeID := b.headShapeID
		for shapeID != NullIndex {
			s := &w.shapes[shapeID]

			if isFastBody {
				// For fast non-bullet bodies the AABB has already been
				// updated in solveContinuous. For fast bullet bodies the AABB
				// will be updated at a later stage.

				// Add to enlarged shapes regardless of AABB changes.
				// Bit-set to keep the move array sorted
				setBit(enlargedSimBitSet, uint32(simIndex))
			} else {
				aabb := computeShapeAABB(s, transform)
				aabb.LowerBound.X -= speculativeDistance
				aabb.LowerBound.Y -= speculativeDistance
				aabb.UpperBound.X += speculativeDistance
				aabb.UpperBound.Y += speculativeDistance
				s.aabb = aabb

				assert(!s.enlargedAABB)

				if !AABBContains(s.fatAABB, aabb) {
					margin := s.aabbMargin
					var fatAABB AABB
					fatAABB.LowerBound.X = aabb.LowerBound.X - margin
					fatAABB.LowerBound.Y = aabb.LowerBound.Y - margin
					fatAABB.UpperBound.X = aabb.UpperBound.X + margin
					fatAABB.UpperBound.Y = aabb.UpperBound.Y + margin
					s.fatAABB = fatAABB

					s.enlargedAABB = true

					// Bit-set to keep the move array sorted
					setBit(enlargedSimBitSet, uint32(simIndex))
				}
			}

			shapeID = s.nextShapeID
		}
	}
}

// solve integrates velocities, solves velocity constraints with graph
// coloring, integrates positions, finalizes bodies, reports events and puts
// islands to sleep (upstream b2Solve, executed serially per the file header).
func (w *World) solve(ctx *stepContext) {
	w.stepIndex++

	// Are there any awake bodies?
	awake := &w.solverSets[awakeSet]
	awakeBodyCount := len(awake.bodySims)
	if awakeBodyCount == 0 {
		// Nothing to simulate. (The tree rebuild already ran inline in
		// updateBroadPhasePairs in this single-threaded port.)
		if debugAsserts {
			assert(w.broadPhase.validateNoEnlarged() == nil)
		}
		return
	}

	// Solve constraints using graph coloring
	{
		// Prepare buffers for bullets
		ctx.bulletBodyCount = 0
		ctx.bulletBodies = w.arena.allocBulletBodies(awakeBodyCount)

		graph := &w.constraintGraph
		ctx.graph = graph

		ctx.sims = awake.bodySims
		ctx.states = awake.bodyStates

		// count colors with work; the overflow color is handled separately
		activeColorCount := 0
		var activeColorIndices [GraphColorCount]int
		for i := range GraphColorCount - 1 {
			occupancyCount := len(graph.colors[i].contactSims) + len(graph.colors[i].jointSims)
			if occupancyCount > 0 {
				activeColorIndices[activeColorCount] = i
				activeColorCount++
			}
		}
		ctx.activeColorCount = activeColorCount

		// prepare for move events (upstream b2BodyMoveEventArray_Resize)
		if cap(w.bodyMoveEvents) < awakeBodyCount {
			w.bodyMoveEvents = make([]BodyMoveEvent, awakeBodyCount)
		} else {
			w.bodyMoveEvents = w.bodyMoveEvents[:awakeBodyCount]
		}

		// Allocate the scalar contact constraints for every color, including
		// overflow, and hand each color its sub-slice (deviation: upstream
		// allocates SIMD-wide constraints for the active colors and scalar
		// constraints for overflow; this port is all-scalar, see
		// contact_solver.go).
		totalContactCount := 0
		for i := range GraphColorCount {
			totalContactCount += len(graph.colors[i].contactSims)
		}

		constraints := w.arena.allocContactConstraints(totalContactCount)
		base := 0
		for i := range GraphColorCount {
			count := len(graph.colors[i].contactSims)
			graph.colors[i].constraints = constraints[base : base+count]
			base += count
		}
		assert(base == totalContactCount)

		// Clear the joint event bit set (upstream sizes it per worker before
		// spawning the solver tasks).
		jointIDCapacity := getIDCapacity(&w.jointIDPool)
		setBitCountAndClear(&w.taskContext.jointStateBitSet, uint32(jointIDCapacity))

		// Split an awake island. This modifies:
		// - world island array and solver set
		// - island indices on bodies, contacts, and joints
		// Upstream enqueues b2SplitIslandTask here; the default (serial) task
		// system runs it immediately, before the solver stages.
		// Note: cannot split islands in parallel with FinalizeBodies.
		if w.splitIslandID != NullIndex {
			w.splitIsland(w.splitIslandID)
			w.taskCount++
		}
		w.splitIslandID = NullIndex

		// -- b2_stagePrepareJoints ----------------------------------------
		for i := range activeColorCount {
			color := &graph.colors[activeColorIndices[i]]
			for j := range color.jointSims {
				w.prepareJoint(&color.jointSims[j], ctx)
			}
		}

		// -- b2_stagePrepareContacts --------------------------------------
		for i := range activeColorCount {
			w.prepareContactsColor(ctx, activeColorIndices[i])
		}

		// Single-threaded overflow work. These constraints don't fit in the
		// graph coloring.
		{
			// b2PrepareOverflowJoints
			overflowColor := &graph.colors[overflowIndex]
			for j := range overflowColor.jointSims {
				w.prepareJoint(&overflowColor.jointSims[j], ctx)
			}
			// b2PrepareOverflowContacts
			w.prepareContactsColor(ctx, overflowIndex)
		}

		subStepCount := ctx.subStepCount
		for range subStepCount {
			// -- b2_stageIntegrateVelocities ------------------------------
			integrateVelocitiesTask(0, awakeBodyCount, ctx)

			// -- b2_stageWarmStart ----------------------------------------
			// b2WarmStartOverflowJoints
			{
				overflowColor := &graph.colors[overflowIndex]
				for j := range overflowColor.jointSims {
					w.warmStartJoint(&overflowColor.jointSims[j], ctx)
				}
			}
			// b2WarmStartOverflowContacts
			w.warmStartContactsColor(ctx, overflowIndex)

			for i := range activeColorCount {
				colorIndex := activeColorIndices[i]
				// joint blocks precede contact blocks within a color stage
				color := &graph.colors[colorIndex]
				for j := range color.jointSims {
					w.warmStartJoint(&color.jointSims[j], ctx)
				}
				w.warmStartContactsColor(ctx, colorIndex)
			}

			// -- b2_stageSolve --------------------------------------------
			useBias := true
			for range solverIterations {
				// Overflow constraints have lower priority
				{
					overflowColor := &graph.colors[overflowIndex]
					for j := range overflowColor.jointSims {
						w.solveJoint(&overflowColor.jointSims[j], ctx, useBias)
					}
				}
				w.solveContactsColor(ctx, overflowIndex, useBias)

				for i := range activeColorCount {
					colorIndex := activeColorIndices[i]
					// b2SolveJointsTask: color joints carry the joint event
					// bookkeeping.
					w.solveJointsColor(ctx, colorIndex, useBias)
					w.solveContactsColor(ctx, colorIndex, useBias)
				}
			}

			// -- b2_stageIntegratePositions -------------------------------
			integratePositionsTask(0, awakeBodyCount, ctx)

			// -- b2_stageRelax --------------------------------------------
			useBias = false
			for range solverRelaxIterations {
				{
					overflowColor := &graph.colors[overflowIndex]
					for j := range overflowColor.jointSims {
						w.solveJoint(&overflowColor.jointSims[j], ctx, useBias)
					}
				}
				w.solveContactsColor(ctx, overflowIndex, useBias)

				for i := range activeColorCount {
					colorIndex := activeColorIndices[i]
					// b2SolveJointsTask (relax): useBias is false so no joint
					// event bookkeeping happens.
					w.solveJointsColor(ctx, colorIndex, useBias)
					w.solveContactsColor(ctx, colorIndex, useBias)
				}
			}
		}

		// -- b2_stageRestitution ------------------------------------------
		// Note: upstream mixes joint blocks into the restitution stages but
		// only graph contact blocks are executed for restitution.
		w.applyRestitutionColor(ctx, overflowIndex)
		for i := range activeColorCount {
			w.applyRestitutionColor(ctx, activeColorIndices[i])
		}

		// -- b2_stageStoreImpulses ----------------------------------------
		w.storeImpulsesColor(ctx, overflowIndex)
		for i := range activeColorCount {
			w.storeImpulsesColor(ctx, activeColorIndices[i])
		}

		// the solver stages ran as one serial task
		w.taskCount++

		// Release the constraint scratch.
		for i := range GraphColorCount {
			graph.colors[i].constraints = nil
		}
		w.arena.freeContactConstraints()

		// Prepare contact, enlarged body, and island bit sets used in body
		// finalization.
		awakeIslandCount := len(awake.islandSims)
		tc := &w.taskContext
		tc.sensorHits = tc.sensorHits[:0]
		setBitCountAndClear(&tc.enlargedSimBitSet, uint32(awakeBodyCount))
		setBitCountAndClear(&tc.awakeIslandBitSet, uint32(awakeIslandCount))
		tc.splitIslandID = NullIndex
		tc.splitSleepTime = 0.0

		// Finalize bodies. Must happen after the constraint solver and after
		// island splitting.
		w.finalizeBodiesTask(0, awakeBodyCount, ctx)
		w.taskCount++
	}

	// Report joint events (upstream: the joint event assembly in b2Solve).
	{
		jointStateBitSet := &w.taskContext.jointStateBitSet
		for k := range jointStateBitSet.blockCount {
			word := jointStateBitSet.bits[k]
			for word != 0 {
				ctz := ctz64(word)
				jointID := int(64*k + ctz)

				j := &w.joints[jointID]
				assert(j.setIndex == awakeSet)

				event := JointEvent{
					JointID: JointID{
						index1:     int32(jointID + 1),
						world0:     w.worldID,
						generation: j.generation,
					},
					UserData: j.userData,
				}

				w.jointEvents = append(w.jointEvents, event)

				// Clear the smallest set bit
				word &= word - 1
			}
		}
	}

	// Report hit events
	{
		assert(len(w.contactHitEvents) == 0)

		threshold := w.hitEventThreshold
		for i := range GraphColorCount {
			color := &w.constraintGraph.colors[i]
			contactCount := len(color.contactSims)
			for j := range contactCount {
				contactSim := &color.contactSims[j]
				if contactSim.simFlags&simEnableHitEvent == 0 {
					continue
				}

				var event ContactHitEvent
				event.ApproachSpeed = threshold

				hit := false
				pointCount := contactSim.manifold.PointCount
				for k := range pointCount {
					mp := &contactSim.manifold.Points[k]
					approachSpeed := -mp.NormalVelocity

					// Need to check total impulse because the point may be
					// speculative and not colliding.
					if approachSpeed > event.ApproachSpeed && mp.TotalNormalImpulse > 0.0 {
						event.ApproachSpeed = approachSpeed
						event.Point = mp.ClipPoint
						hit = true
					}
				}

				if hit {
					event.Normal = contactSim.manifold.Normal

					shapeA := &w.shapes[contactSim.shapeIDA]
					shapeB := &w.shapes[contactSim.shapeIDB]

					event.ShapeIDA = ShapeID{index1: int32(shapeA.id + 1), world0: w.worldID, generation: shapeA.generation}
					event.ShapeIDB = ShapeID{index1: int32(shapeB.id + 1), world0: w.worldID, generation: shapeB.generation}

					c := &w.contacts[contactSim.contactID]
					event.ContactID = ContactID{
						index1:     int32(c.contactID + 1),
						world0:     w.worldID,
						padding:    0,
						generation: c.generation,
					}

					w.contactHitEvents = append(w.contactHitEvents, event)
				}
			}
		}
	}

	// Refit the broad-phase: enlarge proxies and build the move array in
	// deterministic body-sim order. This has to happen before bullets are
	// processed.
	{
		if debugAsserts {
			assert(w.broadPhase.validateNoEnlarged() == nil)
		}

		enlargedBodyBitSet := &w.taskContext.enlargedSimBitSet
		bp := &w.broadPhase

		for k := range enlargedBodyBitSet.blockCount {
			word := enlargedBodyBitSet.bits[k]
			for word != 0 {
				ctz := ctz64(word)
				bodySimIndex := 64*k + ctz

				sim := &awake.bodySims[bodySimIndex]
				b := &w.bodies[sim.bodyID]

				shapeID := b.headShapeID
				if sim.flags&(isBullet|isFast) == isBullet|isFast {
					// Fast bullet bodies don't have their final AABB yet
					for shapeID != NullIndex {
						s := &w.shapes[shapeID]

						// Shape is fast. Its aabb will be enlarged in
						// continuous collision. Update the move array here
						// for determinism because upstream processes bullets
						// below in non-deterministic order.
						bp.bufferMove(s.proxyKey)

						shapeID = s.nextShapeID
					}
				} else {
					for shapeID != NullIndex {
						s := &w.shapes[shapeID]

						// The AABB may not have been enlarged, despite the
						// body being flagged as enlarged. For example, a body
						// with multiple shapes may have not have all shapes
						// enlarged. A fast body may have been flagged as
						// enlarged despite having no shapes enlarged.
						if s.enlargedAABB {
							bp.enlargeProxy(s.proxyKey, s.fatAABB)
							s.enlargedAABB = false
						}

						shapeID = s.nextShapeID
					}
				}

				// Clear the smallest set bit
				word &= word - 1
			}
		}

		if debugAsserts {
			assert(w.broadPhase.validate() == nil)
		}
	}

	if ctx.bulletBodyCount > 0 {
		// Fast bullet bodies (upstream b2BulletBodyTask, run serially here in
		// the deterministic fill order of bulletBodies).
		// Note: a bullet body may be moving slow.
		for i := range ctx.bulletBodyCount {
			w.solveContinuous(ctx.bulletBodies[i])
		}
		w.taskCount++

		// Serially enlarge broad-phase proxies for bullet shapes
		bp := &w.broadPhase
		dynamicTree := &bp.trees[DynamicBody]

		// Upstream notes this loop has non-deterministic order (workers fill
		// bulletBodies concurrently) but it doesn't affect the result. This
		// port keeps the deterministic fill order.
		for i := range ctx.bulletBodyCount {
			bulletBodySim := &awake.bodySims[ctx.bulletBodies[i]]
			if bulletBodySim.flags&enlargeBounds == 0 {
				continue
			}

			// Clear flag
			bulletBodySim.flags &^= enlargeBounds

			bodyID := bulletBodySim.bodyID
			assert(0 <= bodyID && bodyID < len(w.bodies))
			bulletBody := &w.bodies[bodyID]

			shapeID := bulletBody.headShapeID
			for shapeID != NullIndex {
				s := &w.shapes[shapeID]
				if !s.enlargedAABB {
					shapeID = s.nextShapeID
					continue
				}

				// Clear flag
				s.enlargedAABB = false

				proxyKey := s.proxyKey
				proxyID := proxyKeyID(proxyKey)
				assert(proxyKeyType(proxyKey) == DynamicBody)

				// all fast bullet shapes should already be in the move buffer
				assert(containsKey(&bp.moveSet, uint64(proxyKey+1)))

				dynamicTree.EnlargeProxy(proxyID, s.fatAABB)

				shapeID = s.nextShapeID
			}
		}
	}

	// Need to free this even if no bullets got processed.
	w.arena.freeBulletBodies()
	ctx.bulletBodies = nil
	ctx.bulletBodyCount = 0

	// Report sensor hits. This may include bullet sensor hits.
	// Upstream loops over per-worker task contexts; this port has one.
	for _, hit := range w.taskContext.sensorHits {
		sensorShape := &w.shapes[hit.sensorID]
		visitorShape := &w.shapes[hit.visitorID]

		sen := &w.sensors[sensorShape.sensorIndex]
		sen.hits = append(sen.hits, visitor{
			shapeID:    hit.visitorID,
			generation: visitorShape.generation,
		})
	}

	// Island sleeping.
	// This must be done last because putting islands to sleep invalidates the
	// enlarged body bits.
	if w.enableSleep {
		// Collect split island candidate for the next time step. No need to
		// split if sleeping is disabled.
		assert(w.splitIslandID == NullIndex)
		tc := &w.taskContext
		if tc.splitIslandID != NullIndex {
			assert(tc.splitSleepTime > 0.0)
			w.splitIslandID = tc.splitIslandID
		}

		awakeIslandBitSet := &tc.awakeIslandBitSet

		// Need to process in reverse because this moves islands to sleeping
		// solver sets.
		islandSims := awake.islandSims
		count := len(awake.islandSims)
		for islandIndex := count - 1; islandIndex >= 0; islandIndex-- {
			if getBit(awakeIslandBitSet, uint32(islandIndex)) {
				// this island is still awake
				continue
			}

			islandID := islandSims[islandIndex].islandID

			w.trySleepIsland(islandID)
		}

		w.validateSolverSets()
	}
}

// solverIterations is the upstream ITERATIONS constant (solver.c) — the
// number of solve passes per sub-step.
const solverIterations = 1

// solverRelaxIterations is the upstream RELAX_ITERATIONS constant (solver.c)
// — the number of relax passes per sub-step.
const solverRelaxIterations = 1
