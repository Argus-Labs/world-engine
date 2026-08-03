// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/solver.h, src/solver.c
// (the continuous-collision functions live in solver_continuous.go).
//
// STAGE EXECUTION (approved deviation): upstream builds
// b2SolverStage/b2SolverBlock arrays and drives them from b2SolverTask with
// atomic work stealing across workers. This port executes the exact same
// stage sequence, either as serial loops (no worker pool) or as static
// contiguous range dispatches on the internal worker pool (worker_pool.go)
// with a barrier per stage per color — NO stage machine, NO work stealing.
// Within one color every constraint (joint or contact) touches disjoint
// bodies by the graph coloring, so any partition of a color's item list is
// byte-identical to the serial loop; a color's joints and contacts share one
// dispatch (joint items first, matching the upstream block order) to halve
// the barrier count. Mapping (upstream b2SolverStageType and the
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
// In the warm start, solve, relax, restitution and store stages the overflow
// color runs before the active colors (the upstream main thread runs the
// b2*Overflow* calls before executing those per-color stages); the two
// prepare stages run the active colors first and overflow last, per the
// mapping above (upstream b2SolverTask order, solver.c:1090-1108). Joint
// blocks run before contact blocks within a color (upstream block order).
// The overflow color ALWAYS runs serially on the
// dispatching goroutine (worker 0), as upstream's main thread does — overflow
// constraints share bodies, so they cannot be partitioned. Active colors run
// in ascending color-index order. ITERATIONS and RELAX_ITERATIONS are both 1
// upstream; the loops are kept.
//
// Continuous collision (b2SolveContinuous, b2ContinuousQueryCallback) lives
// in solver_continuous.go. The upstream b2BulletBodyTask is the serial bullet
// loop in the solve epilogue below (deviation — see solver_continuous.go);
// each worker's finalizeBodiesTask range gathers its bullet bodies into its
// own taskContext and the ascending-worker concatenation before the bullet
// stage reproduces the serial ascending body-sim index fill exactly, which
// keeps the serial bullet processing deterministic for every worker count.

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
// Deviations from upstream: the stage-machine/atomic machinery (stages,
// blocks, atomicSyncBits, workerCount) is gone — the internal worker pool
// dispatches plain ranges instead, see the file header. The flattened
// joint/contact pointer arrays and the SIMD-wide constraint buffer are not
// needed: the solver iterates the graph colors directly and the scalar
// constraints live per color (graphColor.constraints).
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

	// Array of bullet bodies that need continuous collision handling,
	// filled after the finalize join by concatenating the per-worker
	// taskContext.bulletBodies gathers in ascending worker order (deviation
	// from upstream, which fills it from inside b2FinalizeBodiesTask through
	// an atomic cursor — see taskContext.bulletBodies).
	bulletBodies    []int
	bulletBodyCount int

	activeColorCount int

	enableWarmStarting bool

	// dispatchColorIndex/dispatchUseBias carry the per-color stage arguments
	// into the dispatch closures built once per solve. They live here — not
	// as closure-captured locals — so the serial path allocates nothing (a
	// captured local escapes to the heap even when the closures are never
	// built; the step context is World-owned scratch, see World.stepCtx).
	// Written only by the dispatching goroutine before each forRange; the
	// pool's job publish is the happens-before edge that lets workers read
	// them.
	dispatchColorIndex int
	dispatchUseBias    bool
}

// taskContext is the per-worker scratch (upstream b2TaskContext,
// physics_world.h). The world keeps one per worker in World.taskContexts;
// worker 0 is the Step goroutine and the serial path uses only slot 0. Each
// step, every stage sizes/clears exactly the slots its dispatch engages
// (forRangeWorkers of the stage's item count and grain — the same pure
// function its merge uses), so a merge never sees stale bits or mismatched
// block counts in the slots it reads; slots beyond a stage's engaged count
// keep stale content, which is harmless because that stage neither writes
// nor reads them that step.
//
// Deviation from upstream: b2SensorTaskContext (a separate per-worker struct
// whose only member is an event bitset) is folded in here as sensorEventBits,
// so there is exactly one per-worker context array. The bullet body gather
// list is also per-worker here (bulletBodies) instead of upstream's shared
// array with an atomic cursor — see the field comment.
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

	// Sensors whose overlap set changed during the sensor pass (bit index =
	// sensor array index). Deviation from upstream: this is
	// b2SensorTaskContext.eventBits folded into the one per-worker context
	// (see the struct comment). Written by sensorTask, merged into slot 0 in
	// ascending worker order before the publish drain (sensor.go).
	sensorEventBits bitSet

	// Bullet bodies gathered by finalizeBodiesTask (values = awake body sim
	// indices). Deviation from upstream, which appends to one shared
	// stepContext array through an atomic cursor and documents the resulting
	// order as non-deterministic: per-worker lists concatenated in ascending
	// worker order reproduce the serial ascending-sim-index fill exactly,
	// which the bullet solve and enlarge loops rely on (solver.go). Grows and
	// never shrinks; reset to length zero each step.
	bulletBodies []int

	// Per-worker scratch for solveContinuous. Kept on the task context so
	// passing its address through the tree query's `any` context parameter
	// does not force a fresh heap allocation per fast body per step. Every
	// field consumed by a sweep is (re)initialized at the top of
	// solveContinuous. Placed last so this large, cold-between-sweeps block
	// does not shift the hot fields above onto extra cache lines.
	continuous continuousContext
}

func createTaskContext() taskContext {
	return taskContext{
		contactStateBitSet: createBitSet(1024),
		enlargedSimBitSet:  createBitSet(256),
		awakeIslandBitSet:  createBitSet(256),
		jointStateBitSet:   createBitSet(1024),
		splitIslandID:      NullIndex,
		sensorEventBits:    createBitSet(64),
	}
}

func destroyTaskContext(tc *taskContext) {
	destroyBitSet(&tc.contactStateBitSet)
	destroyBitSet(&tc.enlargedSimBitSet)
	destroyBitSet(&tc.awakeIslandBitSet)
	destroyBitSet(&tc.jointStateBitSet)
	destroyBitSet(&tc.sensorEventBits)
	tc.bulletBodies = nil
	*tc = taskContext{}
}

// solveJointsColor solves the joints [startIndex, endIndex) of one active
// graph color and performs the joint event bookkeeping (upstream
// b2SolveJointsTask). Note that upstream overflow joints run through
// b2SolveOverflowJoints, which has no event bookkeeping — the overflow loops
// in solve call solveJoint directly to match.
//
// The getBit read below makes the "already flagged" dedup PER-WORKER instead
// of global. That cannot change any result: the per-worker bit sets are
// bit-OR merged before the joint event drain (the emitted SET is identical),
// a joint lives in exactly one color at a fixed index so the same worker
// visits it every sub-step (the partition is a pure function of the fixed
// color size), and getJointReaction is pure (it reads impulse fields and
// returns two floats), so even a repeated evaluation could not diverge
// state.
func (w *World) solveJointsColor(ctx *stepContext, colorIndex int, useBias bool, workerIndex, startIndex, endIndex int) {
	color := &ctx.graph.colors[colorIndex]
	jointStateBitSet := &w.taskContexts[workerIndex].jointStateBitSet

	assert(0 <= startIndex && startIndex <= endIndex && endIndex <= len(color.jointSims))

	for i := startIndex; i < endIndex; i++ {
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

// prepareJointsColorRange prepares the joints [startIndex, endIndex) of one
// graph color (upstream b2PrepareJointsTask over one color's joint blocks).
// Writes are joint-owned, so any partition is byte-identical.
func (w *World) prepareJointsColorRange(ctx *stepContext, colorIndex, startIndex, endIndex int) {
	color := &ctx.graph.colors[colorIndex]
	assert(0 <= startIndex && startIndex <= endIndex && endIndex <= len(color.jointSims))
	for i := startIndex; i < endIndex; i++ {
		w.prepareJoint(&color.jointSims[i], ctx)
	}
}

// warmStartJointsColorRange warm-starts the joints [startIndex, endIndex) of
// one graph color (upstream b2WarmStartJointsTask over one color's joint
// blocks). Body writes are disjoint within a color by the graph coloring.
func (w *World) warmStartJointsColorRange(ctx *stepContext, colorIndex, startIndex, endIndex int) {
	color := &ctx.graph.colors[colorIndex]
	assert(0 <= startIndex && startIndex <= endIndex && endIndex <= len(color.jointSims))
	for i := startIndex; i < endIndex; i++ {
		w.warmStartJoint(&color.jointSims[i], ctx)
	}
}

// warmStartColorTask warm-starts the items [startIndex, endIndex) of one
// color's combined item list: the color's joints (items [0, jointCount))
// followed by its contacts (upstream: joint blocks precede contact blocks
// within a color stage). Joints and contacts of one color are mutually
// body-disjoint by the graph coloring — addContactToGraph and
// assignJointColor test the same per-color bodySet — so one barrier covers
// both and any partition of the combined list is byte-identical to the
// serial full-range call.
func (w *World) warmStartColorTask(ctx *stepContext, colorIndex, startIndex, endIndex int) {
	jointCount := len(ctx.graph.colors[colorIndex].jointSims)
	if startIndex < jointCount {
		w.warmStartJointsColorRange(ctx, colorIndex, startIndex, min(endIndex, jointCount))
	}
	if endIndex > jointCount {
		w.warmStartContactsColor(ctx, colorIndex, max(startIndex, jointCount)-jointCount, endIndex-jointCount)
	}
}

// solveColorTask solves the items [startIndex, endIndex) of one color's
// combined joint+contact item list (see warmStartColorTask for the layout
// and the body-disjointness argument). Used for both the solve
// (useBias=true) and relax (useBias=false) stages.
func (w *World) solveColorTask(ctx *stepContext, colorIndex int, useBias bool, workerIndex, startIndex, endIndex int) {
	jointCount := len(ctx.graph.colors[colorIndex].jointSims)
	if startIndex < jointCount {
		w.solveJointsColor(ctx, colorIndex, useBias, workerIndex, startIndex, min(endIndex, jointCount))
	}
	if endIndex > jointCount {
		w.solveContactsColor(ctx, colorIndex, useBias, max(startIndex, jointCount)-jointCount, endIndex-jointCount)
	}
}

// solverBodyGrain is the dispatch grain for body-range solver stages
// (integrate velocities/positions, finalize): the minimum awake bodies per
// engaged worker (upstream passes minRange 64 when enqueueing its body
// parallel-fors). forRangeWorkers caps the engaged worker count at
// n/solverBodyGrain, so per-worker ranges never shrink below a useful size —
// no worker-count term is needed. Grain affects performance only — results
// are partition-independent. Measured on BenchmarkStepMixedRain (Apple M-class
// arm64, 2026-08): with the capped engagement this constant keeps
// Workers_N within noise of Workers_1 at 500/1000 bodies and >= 1.7x at 5000.
const solverBodyGrain = 64

// solverColorGrain is the dispatch grain for the HEAVY per-color constraint
// stages — solve and relax: the minimum items (joints+contacts of one color)
// per engaged worker (upstream uses block sizes of 4 for joint and
// SIMD-contact blocks; this port dispatches whole ranges, so the threshold is
// higher to amortize the barrier). Grain affects performance only; measured
// together with solverBodyGrain above.
//
// Those two stages run solverIterations/solverRelaxIterations passes per
// sub-step over the full constraint solve, so their per-item work dwarfs one
// barrier and a fine grain pays off. Wall-clock stage instrumentation
// (MixedRain 5000 bodies, 600 steps, Apple M5 Max, 18 workers, 2026-08):
//
//	stage              serial ms   parallel ms
//	solveIterations      0.8977      0.5739   (1.56x — parallel wins)
//	relaxIterations      0.8864      0.5697   (1.56x — parallel wins)
const solverColorGrain = 32

// solverLightColorGrain is the dispatch grain for the LIGHT per-color
// constraint stages — prepare joints, prepare contacts, warm start, apply
// restitution and store impulses. Each runs a single cheap pass per color
// (one pass per solve for prepare/restitution/store, one per sub-step for
// warm start), so the whole stage can cost less than the ~6-10us barrier a
// dispatch pays. On the same instrumentation run all four of the measured
// light stages LOST to serial at the shared grain of 32:
//
//	stage              serial ms   parallel ms
//	warmStartContacts    0.1444      0.2387   (loses)
//	prepareContacts      0.0603      0.0728   (loses)
//	applyRestitution     0.0339      0.0575   (loses)
//	storeImpulses        0.0368      0.0540   (loses)
//
// A larger grain fixes that without a special case: forRangeWorkers keeps
// these stages on the dispatching goroutine until a color is genuinely big
// enough for the work to outweigh the barrier, while still parallelizing huge
// colors.
//
// The value was swept end to end (candidates 32/64/128/256/512/1024/2048 and
// 1<<30, runs interleaved across candidates so build order cannot masquerade
// as an effect, 10 reps of -benchtime=50x each, Workers_18):
//
//   - StepPyramid (colors of 74-100 items): every candidate >= 64 gains
//     ~5.5% over 32; they are statistically tied with each other.
//   - StepMixedRain 1000 and 5000 bodies (colors up to 356 and 1838 items):
//     no candidate differs from 32 — parallelizing these stages is worth
//     nothing even on the largest color measured.
//   - A denser pyramid (colors of 322-409 items, probe scene) separates the
//     small candidates: 64 gains only 8.4% where 256/1024/1<<30 gain 12-14%.
//     A grain of 64 wins the 20-row pyramid merely because its colors happen
//     to fall under 2*64; it re-engages 6 workers per color as soon as the
//     colors grow, and loses the barrier back.
//
// So every candidate >= 256 is tied on measurement, and 64/128 are fragile.
// The tie is broken by cost: the instrumented warm start above is ~3.7 ns per
// item (0.1444 ms over 4 sub-steps x ~9.7k contacts), so a worker needs
// thousands of items before its share covers a 6-10 us barrier. 1024 is the
// smallest tied candidate at that scale, and it still lets a color of 2048+
// items dispatch.
//
// INVARIANT NOTE: retuning these stages is safe for the presize/merge bound
// documented in taskContext because none of them write per-worker state —
// every write is item-owned. The only per-color stage with a per-worker
// output is solve/relax (taskContext.jointStateBitSet, written by
// solveJointsColor), and maxColorWorkers in solve() derives its bound from
// the SAME (itemCount, solverColorGrain) pair that dispatch still uses.
const solverLightColorGrain = 1024

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
// b2FinalizeBodiesTask; workerIndex is upstream's threadIndex).
//
// Concurrency contract: every write is owned by the body at simIndex (its
// state, sim, body struct, shapes and bodyMoveEvents slot) or by the worker
// (the taskContext bit sets, sensorHits, bulletBodies and split candidate);
// island structs are only read. Disjoint ranges may therefore run
// concurrently, and the per-worker outputs are merged after the join — bit
// sets by order-free bit-OR, bulletBodies and sensorHits by ascending-worker
// concatenation (== ascending body-sim order because ranges are contiguous
// ascending), the split candidate by an ascending-worker strict-> reduction.
func (w *World) finalizeBodiesTask(startIndex, endIndex, workerIndex int, ctx *stepContext) {
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

	tc := &w.taskContexts[workerIndex]
	enlargedSimBitSet := &tc.enlargedSimBitSet
	awakeIslandBitSet := &tc.awakeIslandBitSet

	enableContinuous := w.enableContinuous

	speculativeDistance := SpeculativeDistance

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

				// Store in the worker's fast array for the continuous
				// collision stage (deviation from upstream's shared
				// atomic-cursor array: the ascending-worker concatenation
				// after the join reproduces the serial ascending-sim-index
				// fill exactly — see taskContext.bulletBodies).
				if sim.flags&isBullet != 0 {
					tc.bulletBodies = append(tc.bulletBodies, simIndex)
				} else {
					w.solveContinuous(simIndex, tc)
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
// islands to sleep (upstream b2Solve; stages dispatch per the file header,
// everything order-sensitive runs on the Step goroutine between barriers).
func (w *World) solve(ctx *stepContext) {
	w.stepIndex++

	// Are there any awake bodies?
	awake := &w.solverSets[awakeSet]
	awakeBodyCount := len(awake.bodySims)
	if awakeBodyCount == 0 {
		// Nothing to simulate. (The tree rebuild already ran inline in
		// updateBroadPhasePairs in this port.)
		if debugAsserts {
			assert(w.broadPhase.validateNoEnlarged() == nil)
		}
		return
	}

	// Per-stage engaged worker counts — pure functions of (item count, grain,
	// workerCount), so INVARIANT: dispatch bound == presize bound == merge
	// bound at every stage. finalizeWorkers bounds every output written by
	// the finalize dispatch (enlargedSimBitSet, awakeIslandBitSet, sensorHits,
	// bulletBodies, split candidates); maxColorWorkers bounds the
	// jointStateBitSet written by the per-color solve dispatches (each color
	// engages forRangeWorkers(itemCount, solverColorGrain, ...) workers, so
	// the max over active colors covers them all; overflow runs serially on
	// slot 0, hence the floor of 1). Slots >= a stage's bound are never
	// written this step and never merged this step, so stale content in them
	// is harmless.
	finalizeWorkers := forRangeWorkers(awakeBodyCount, solverBodyGrain, w.workerCount)
	maxColorWorkers := 1

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
				// occupancyCount is exactly the itemCount the warm-start and
				// solve dispatches below hand to forRange for this color.
				maxColorWorkers = max(maxColorWorkers,
					forRangeWorkers(occupancyCount, solverColorGrain, w.workerCount))
			}
		}
		ctx.activeColorCount = activeColorCount

		// prepare for move events (upstream b2BodyMoveEventArray_Resize).
		// Reset discipline: length only — finalizeBodiesTask writes every slot
		// before BodyEvents can observe it, which the previous
		// reslice-when-large-enough branch already relied on.
		w.bodyMoveEvents = growScratch(w.bodyMoveEvents, awakeBodyCount)

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
		// spawning the solver tasks). INVARIANT: presize bound == dispatch
		// bound == merge bound == maxColorWorkers — the per-color solve
		// dispatches engage at most that many workers, so slots >=
		// maxColorWorkers are never written this step and never merged this
		// step; stale bits or block counts in them are harmless.
		jointIDCapacity := getIDCapacity(&w.jointIDPool)
		for k := range maxColorWorkers {
			setBitCountAndClear(&w.taskContexts[k].jointStateBitSet, uint32(jointIDCapacity))
		}

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

		// Dispatch plumbing, built once per solve so the per-color dispatch
		// sites below allocate nothing per call: the color/bias arguments
		// travel through ctx.dispatchColorIndex/ctx.dispatchUseBias — fields
		// of the World-owned step context, NOT closure-captured locals, so
		// the serial path stays at zero heap allocations (a captured local
		// would escape even when the closures are never built) — written by
		// the dispatching goroutine before each forRange (the pool's job
		// publish is the happens-before edge). Nil — and never touched —
		// when the world has no pool. This block moves only ints, bools and
		// funcs; it adds no floating-point arithmetic.
		var (
			integrateVelocitiesFn func(workerIndex, startIndex, endIndex int)
			integratePositionsFn  func(workerIndex, startIndex, endIndex int)
			prepareJointsFn       func(workerIndex, startIndex, endIndex int)
			prepareContactsFn     func(workerIndex, startIndex, endIndex int)
			warmStartColorFn      func(workerIndex, startIndex, endIndex int)
			solveColorFn          func(workerIndex, startIndex, endIndex int)
			restitutionFn         func(workerIndex, startIndex, endIndex int)
			storeImpulsesFn       func(workerIndex, startIndex, endIndex int)
			finalizeFn            func(workerIndex, startIndex, endIndex int)
		)
		if w.pool != nil {
			integrateVelocitiesFn = func(_, startIndex, endIndex int) {
				integrateVelocitiesTask(startIndex, endIndex, ctx)
			}
			integratePositionsFn = func(_, startIndex, endIndex int) {
				integratePositionsTask(startIndex, endIndex, ctx)
			}
			prepareJointsFn = func(_, startIndex, endIndex int) {
				w.prepareJointsColorRange(ctx, ctx.dispatchColorIndex, startIndex, endIndex)
			}
			prepareContactsFn = func(_, startIndex, endIndex int) {
				w.prepareContactsColor(ctx, ctx.dispatchColorIndex, startIndex, endIndex)
			}
			warmStartColorFn = func(_, startIndex, endIndex int) {
				w.warmStartColorTask(ctx, ctx.dispatchColorIndex, startIndex, endIndex)
			}
			solveColorFn = func(workerIndex, startIndex, endIndex int) {
				w.solveColorTask(ctx, ctx.dispatchColorIndex, ctx.dispatchUseBias, workerIndex, startIndex, endIndex)
			}
			restitutionFn = func(_, startIndex, endIndex int) {
				w.applyRestitutionColor(ctx, ctx.dispatchColorIndex, startIndex, endIndex)
			}
			storeImpulsesFn = func(_, startIndex, endIndex int) {
				w.storeImpulsesColor(ctx, ctx.dispatchColorIndex, startIndex, endIndex)
			}
			finalizeFn = func(workerIndex, startIndex, endIndex int) {
				w.finalizeBodiesTask(startIndex, endIndex, workerIndex, ctx)
			}
		}

		// -- b2_stagePrepareJoints ----------------------------------------
		for i := range activeColorCount {
			//nolint:gosec // G602: activeColorIndices is a local [GraphColorCount]int filled above with color indices i < GraphColorCount-1, and activeColorCount counts exactly those entries, so i < activeColorCount indexes a written slot holding a valid graph.colors index.
			colorIndex := activeColorIndices[i]
			jointCount := len(graph.colors[colorIndex].jointSims)
			if w.pool == nil {
				w.prepareJointsColorRange(ctx, colorIndex, 0, jointCount)
			} else {
				ctx.dispatchColorIndex = colorIndex
				w.pool.forRange(jointCount, solverLightColorGrain, prepareJointsFn)
			}
		}

		// -- b2_stagePrepareContacts --------------------------------------
		for i := range activeColorCount {
			//nolint:gosec // G602: same bound as the other activeColorIndices reads; the array is a local [GraphColorCount]int filled above with indices i < GraphColorCount-1 and activeColorCount entries.
			colorIndex := activeColorIndices[i]
			contactCount := len(graph.colors[colorIndex].contactSims)
			if w.pool == nil {
				w.prepareContactsColor(ctx, colorIndex, 0, contactCount)
			} else {
				ctx.dispatchColorIndex = colorIndex
				w.pool.forRange(contactCount, solverLightColorGrain, prepareContactsFn)
			}
		}

		// Overflow work, always on the dispatching goroutine. These
		// constraints don't fit in the graph coloring and can share bodies,
		// so they cannot be partitioned.
		{
			// b2PrepareOverflowJoints
			overflowColor := &graph.colors[overflowIndex]
			for j := range overflowColor.jointSims {
				w.prepareJoint(&overflowColor.jointSims[j], ctx)
			}
			// b2PrepareOverflowContacts
			w.prepareContactsColor(ctx, overflowIndex, 0, len(overflowColor.contactSims))
		}

		subStepCount := ctx.subStepCount
		for range subStepCount {
			// -- b2_stageIntegrateVelocities ------------------------------
			if w.pool == nil {
				integrateVelocitiesTask(0, awakeBodyCount, ctx)
			} else {
				w.pool.forRange(awakeBodyCount, solverBodyGrain, integrateVelocitiesFn)
			}

			// -- b2_stageWarmStart ----------------------------------------
			// Overflow first, on the dispatching goroutine, exactly like
			// upstream's main thread.
			{
				// b2WarmStartOverflowJoints
				overflowColor := &graph.colors[overflowIndex]
				for j := range overflowColor.jointSims {
					w.warmStartJoint(&overflowColor.jointSims[j], ctx)
				}
				// b2WarmStartOverflowContacts
				w.warmStartContactsColor(ctx, overflowIndex, 0, len(overflowColor.contactSims))
			}

			for i := range activeColorCount {
				//nolint:gosec // G602: same bound as the other activeColorIndices reads; the array is a local [GraphColorCount]int filled above with indices i < GraphColorCount-1 and activeColorCount entries.
				colorIndex := activeColorIndices[i]
				// joint blocks precede contact blocks within a color stage
				color := &graph.colors[colorIndex]
				itemCount := len(color.jointSims) + len(color.contactSims)
				if w.pool == nil {
					w.warmStartColorTask(ctx, colorIndex, 0, itemCount)
				} else {
					ctx.dispatchColorIndex = colorIndex
					w.pool.forRange(itemCount, solverLightColorGrain, warmStartColorFn)
				}
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
					w.solveContactsColor(ctx, overflowIndex, useBias, 0, len(overflowColor.contactSims))
				}

				for i := range activeColorCount {
					//nolint:gosec // G602: same bound as the other activeColorIndices reads; the array is a local [GraphColorCount]int filled above with indices i < GraphColorCount-1 and activeColorCount entries.
					colorIndex := activeColorIndices[i]
					// b2SolveJointsTask: color joints carry the joint event
					// bookkeeping.
					color := &graph.colors[colorIndex]
					itemCount := len(color.jointSims) + len(color.contactSims)
					if w.pool == nil {
						w.solveColorTask(ctx, colorIndex, useBias, 0, 0, itemCount)
					} else {
						ctx.dispatchColorIndex = colorIndex
						ctx.dispatchUseBias = useBias
						w.pool.forRange(itemCount, solverColorGrain, solveColorFn)
					}
				}
			}

			// -- b2_stageIntegratePositions -------------------------------
			if w.pool == nil {
				integratePositionsTask(0, awakeBodyCount, ctx)
			} else {
				w.pool.forRange(awakeBodyCount, solverBodyGrain, integratePositionsFn)
			}

			// -- b2_stageRelax --------------------------------------------
			useBias = false
			for range solverRelaxIterations {
				{
					overflowColor := &graph.colors[overflowIndex]
					for j := range overflowColor.jointSims {
						w.solveJoint(&overflowColor.jointSims[j], ctx, useBias)
					}
					w.solveContactsColor(ctx, overflowIndex, useBias, 0, len(overflowColor.contactSims))
				}

				for i := range activeColorCount {
					//nolint:gosec // G602: same bound as the other activeColorIndices reads; the array is a local [GraphColorCount]int filled above with indices i < GraphColorCount-1 and activeColorCount entries.
					colorIndex := activeColorIndices[i]
					// b2SolveJointsTask (relax): useBias is false so no joint
					// event bookkeeping happens.
					color := &graph.colors[colorIndex]
					itemCount := len(color.jointSims) + len(color.contactSims)
					if w.pool == nil {
						w.solveColorTask(ctx, colorIndex, useBias, 0, 0, itemCount)
					} else {
						ctx.dispatchColorIndex = colorIndex
						ctx.dispatchUseBias = useBias
						w.pool.forRange(itemCount, solverColorGrain, solveColorFn)
					}
				}
			}
		}

		// -- b2_stageRestitution ------------------------------------------
		// Note: upstream mixes joint blocks into the restitution stages but
		// only graph contact blocks are executed for restitution.
		w.applyRestitutionColor(ctx, overflowIndex, 0, len(graph.colors[overflowIndex].contactSims))
		for i := range activeColorCount {
			//nolint:gosec // G602: same bound as the other activeColorIndices reads; the array is a local [GraphColorCount]int filled above with indices i < GraphColorCount-1 and activeColorCount entries.
			colorIndex := activeColorIndices[i]
			contactCount := len(graph.colors[colorIndex].contactSims)
			if w.pool == nil {
				w.applyRestitutionColor(ctx, colorIndex, 0, contactCount)
			} else {
				ctx.dispatchColorIndex = colorIndex
				w.pool.forRange(contactCount, solverLightColorGrain, restitutionFn)
			}
		}

		// -- b2_stageStoreImpulses ----------------------------------------
		w.storeImpulsesColor(ctx, overflowIndex, 0, len(graph.colors[overflowIndex].contactSims))
		for i := range activeColorCount {
			//nolint:gosec // G602: same bound as the other activeColorIndices reads; the array is a local [GraphColorCount]int filled above with indices i < GraphColorCount-1 and activeColorCount entries.
			colorIndex := activeColorIndices[i]
			contactCount := len(graph.colors[colorIndex].contactSims)
			if w.pool == nil {
				w.storeImpulsesColor(ctx, colorIndex, 0, contactCount)
			} else {
				ctx.dispatchColorIndex = colorIndex
				w.pool.forRange(contactCount, solverLightColorGrain, storeImpulsesFn)
			}
		}

		// the solver stages ran as one serial task
		w.taskCount++

		// Release the constraint scratch.
		for i := range GraphColorCount {
			graph.colors[i].constraints = nil
		}
		w.arena.freeContactConstraints()

		// Prepare the enlarged body and island bit sets, sensor hit and bullet
		// gathers, and split candidates used in body finalization. INVARIANT:
		// presize bound == dispatch bound == merge bound == finalizeWorkers
		// (the finalize forRange below engages exactly that many workers).
		// Slots >= finalizeWorkers are never written this step and never
		// merged this step, so stale content in them is harmless.
		awakeIslandCount := len(awake.islandSims)
		for k := range finalizeWorkers {
			tc := &w.taskContexts[k]
			tc.sensorHits = tc.sensorHits[:0]
			tc.bulletBodies = tc.bulletBodies[:0]
			setBitCountAndClear(&tc.enlargedSimBitSet, uint32(awakeBodyCount))
			setBitCountAndClear(&tc.awakeIslandBitSet, uint32(awakeIslandCount))
			tc.splitIslandID = NullIndex
			tc.splitSleepTime = 0.0
		}

		// Finalize bodies. Must happen after the constraint solver and after
		// island splitting.
		if w.pool == nil {
			w.finalizeBodiesTask(0, awakeBodyCount, 0, ctx)
		} else {
			w.pool.forRange(awakeBodyCount, solverBodyGrain, finalizeFn)
		}
		w.taskCount++
	}

	// Report joint events (upstream: the joint event assembly in b2Solve).
	// First merge the per-worker joint state bits into slot 0 in ascending
	// worker order (upstream: the bit-OR loop before the joint event scan).
	// Bit-OR is order-free. INVARIANT: merge bound == presize bound ==
	// dispatch bound == maxColorWorkers — only slots presized THIS step are
	// unioned, satisfying inPlaceUnion's equal-blockCount contract (slots >=
	// maxColorWorkers may hold stale block counts from earlier steps, but
	// they were never written this step and are not merged). The drain below
	// walks set bits in ascending joint-id order, so the emitted event order
	// is identical for every worker count.
	{
		jointStateBitSet := &w.taskContexts[0].jointStateBitSet
		for k := 1; k < maxColorWorkers; k++ {
			inPlaceUnion(jointStateBitSet, &w.taskContexts[k].jointStateBitSet)
		}
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

		// Merge the per-worker enlarged-sim bits into slot 0 in ascending
		// worker order (upstream: the bit-OR loop before the refit in
		// b2Solve). INVARIANT: merge bound == presize bound == dispatch
		// bound == finalizeWorkers; only slots presized THIS step are
		// unioned (inPlaceUnion asserts equal block counts), and slots >=
		// finalizeWorkers were never written this step. The drain below
		// walks set bits in ascending body-sim order, which keeps the move
		// array deterministic — the cornerstone of the next step's
		// pair-finding order.
		enlargedBodyBitSet := &w.taskContexts[0].enlargedSimBitSet
		for k := 1; k < finalizeWorkers; k++ {
			inPlaceUnion(enlargedBodyBitSet, &w.taskContexts[k].enlargedSimBitSet)
		}

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

	// Gather the per-worker bullet lists into the step context in ascending
	// worker order. Ranges are contiguous ascending, so this concatenation
	// equals the ascending body-sim index fill of the serial engine exactly
	// (deviation from upstream's shared atomic-cursor array — see
	// taskContext.bulletBodies). ctx.bulletBodies has capacity for every
	// awake body, so the copy cannot overflow. INVARIANT: merge bound ==
	// presize bound == dispatch bound == finalizeWorkers (slots beyond it
	// were never written this step).
	for k := range finalizeWorkers {
		for _, simIndex := range w.taskContexts[k].bulletBodies {
			ctx.bulletBodies[ctx.bulletBodyCount] = simIndex
			ctx.bulletBodyCount++
		}
	}

	// Merge the per-worker sensor hits into slot 0 in ascending worker order
	// BEFORE the bullet stage: ranges are contiguous ascending, so this
	// reproduces the serial engine's ascending body-sim append order, and
	// the bullet stage below appends its own hits to slot 0 after the
	// finalize hits — exactly the serial order (upstream drains the
	// per-worker lists in the same worker order after the bullet task).
	// INVARIANT: merge bound == presize bound == dispatch bound ==
	// finalizeWorkers (slots beyond it were never written this step; any
	// stale hits they hold from earlier steps are never read).
	{
		tc0 := &w.taskContexts[0]
		for k := 1; k < finalizeWorkers; k++ {
			tc0.sensorHits = append(tc0.sensorHits, w.taskContexts[k].sensorHits...)
		}
	}

	if ctx.bulletBodyCount > 0 {
		// Fast bullet bodies (upstream b2BulletBodyTask; STAYS SERIAL in
		// this port, in the deterministic fill order of bulletBodies — see
		// the deviation note in solver_continuous.go).
		// Note: a bullet body may be moving slow.
		for i := range ctx.bulletBodyCount {
			w.solveContinuous(ctx.bulletBodies[i], &w.taskContexts[0])
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

	// Report sensor hits. This may include bullet sensor hits. Slot 0 holds
	// the merged list: the finalize hits were concatenated in ascending
	// worker order before the bullet stage and the bullet hits were appended
	// after them, reproducing the serial append order exactly.
	for _, hit := range w.taskContexts[0].sensorHits {
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
		// split if sleeping is disabled. The per-worker candidates are
		// reduced in ascending worker order with the SAME strict > as the
		// per-body scan in finalizeBodiesTask: ranges are contiguous
		// ascending, so the first strict maximum across workers is the first
		// strict maximum of the serial ascending body scan — the choice is
		// identical for every worker count (upstream needs an extra
		// island-id tie-break only because of work stealing, which this port
		// has none of). INVARIANT: reduction bound == presize bound ==
		// dispatch bound == finalizeWorkers (slots beyond it were never
		// written this step; stale candidates in them are never read).
		assert(w.splitIslandID == NullIndex)
		splitIslandID := NullIndex
		splitSleepTime := 0.0
		for k := range finalizeWorkers {
			tc := &w.taskContexts[k]
			if tc.splitIslandID == NullIndex {
				continue
			}
			assert(tc.splitSleepTime > 0.0)
			if tc.splitSleepTime > splitSleepTime {
				splitIslandID = tc.splitIslandID
				splitSleepTime = tc.splitSleepTime
			}
		}
		if splitIslandID != NullIndex {
			w.splitIslandID = splitIslandID
		}

		// Merge the per-worker awake-island bits into slot 0 in ascending
		// worker order (upstream: the bit-OR loop before the island sleep
		// sweep in b2Solve). Bit-OR is order-free. INVARIANT: merge bound ==
		// presize bound == dispatch bound == finalizeWorkers — only slots
		// presized THIS step are unioned (inPlaceUnion asserts equal block
		// counts); slots beyond it were never written this step.
		awakeIslandBitSet := &w.taskContexts[0].awakeIslandBitSet
		for k := 1; k < finalizeWorkers; k++ {
			inPlaceUnion(awakeIslandBitSet, &w.taskContexts[k].awakeIslandBitSet)
		}

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
