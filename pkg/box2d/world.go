// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/physics_world.c, src/physics_world.h
// (world struct, create/destroy, id validation and lookup helpers; Step,
// queries, events and debug draw arrive with later stages).
//
// DESIGN DEVIATION (approved): no world registry, owner tokens instead.
//
// Upstream keeps a global b2World b2_worlds[B2_MAX_WORLDS] array and resolves
// every id's world0 field through it (b2GetWorldFromId/b2GetWorld). This port
// forbids package-level mutable simulation state, so NewWorld returns a *World
// and every b2World_*/b2Body_*/b2Shape_* function becomes a method on *World.
//
// Because the world is the receiver rather than something looked up from the
// id, the id itself must carry proof of ownership: each World takes a distinct
// owner token from nextWorldToken at creation and stamps it into the world0
// field of every BodyID/ShapeID/ChainID/JointID/ContactID it hands out. Every
// validator (IsBodyValid and friends) and every internal full-id lookup
// compares id.world0 against the world's own token in addition to the index,
// generation and inUse checks, so applying world A's id to world B is rejected
// instead of silently addressing an unrelated slot in B. The destructive entry
// points (DestroyBody/DestroyShape/DestroyChain/DestroyJoint) reject foreign
// ids at runtime, not only under debugAsserts, because destroying the wrong
// object is unrecoverable.
//
// The token is creation-time bookkeeping only: it never reaches the solver,
// the broad phase or any golden hash, so it cannot affect determinism.
// Upstream's B2_MAX_WORLDS bound on world0 is dropped — it existed to keep
// world0 inside the registry array, and the token equality check is strictly
// stronger. See nextWorldToken for the wrap-around behavior.
//
// Other deviations from upstream:
//   - Task system ported as an internal goroutine pool (worker_pool.go):
//     upstream's user-supplied enqueueTaskFcn/finishTaskFcn/userTaskContext/
//     userTreeTask callbacks are NOT exposed — WorldDef.WorkerCount selects
//     the worker count and the World owns its workers. activeTaskCount has
//     no counterpart; taskCount counts stages, not per-worker tasks, so
//     Counters.TaskCount is identical for every worker count. The per-worker
//     b2TaskContext array is ported as World.taskContexts (the serial path
//     uses slot 0 only), and the separate b2SensorTaskContext array is folded
//     into taskContext.sensorEventBits (see sensor.go and solver.go). Unlike
//     upstream, simulation results are byte-identical for every worker count.
//   - b2InitializeContactRegisters is a call-parity no-op: the register
//     table is a package-level immutable literal (contact.go).
//   - Counters.ByteCount is 0: upstream b2GetByteCount tracks global heap
//     allocations, which has no Go counterpart.
//   - Counters.StackUsed counts arena elements, not bytes (see arena.go).
//   - b2ValidateConnectivity/b2ValidateSolverSets/b2ValidateContacts are
//     no-ops, matching upstream release builds (B2_ENABLE_VALIDATION off).
//     Stage E7 may port the full validators behind debugAsserts.
//   - b2World_DumpMemoryStats and b2World_Dump (file I/O debug helpers) are
//     not ported.

package box2d

import (
	"math"
	"sync/atomic"
)

// worldTokenCount is the number of distinct world owner tokens. The world0
// field of every id is a uint16 and WorldID stores index1 == token+1, so a
// token must stay below 0xFFFF for index1 to remain non-zero (index1 == 0 is
// the null id).
const worldTokenCount = 0xFFFF

// worldTokenCounter is the source of world owner tokens. It replaces the index
// into upstream's b2_worlds[B2_MAX_WORLDS] array: with no global registry, the
// token stamped into an id is the only thing that binds that id to the world
// that minted it.
//
// This is not simulation state. It is read once per NewWorld and never enters
// the solver, the broad phase, the event data used by callers for stepping, or
// any golden hash, so the package's determinism guarantee is untouched. It is
// atomic so concurrent NewWorld calls still get distinct tokens; the ban on
// goroutines and shared mutable state applies to the simulation path only.
//

var worldTokenCounter atomic.Uint64

// nextWorldToken returns the owner token for a newly created world.
//
// WRAP-AROUND: tokens are handed out modulo worldTokenCount, so the 65536th
// world created in a process reuses the first world's token. Wrapping is
// deliberate. A reused token only weakens detection, it never corrupts: an id
// that survives the token check must still pass the unchanged index,
// generation and inUse checks, so anything the receiving world accepts still
// names a live slot it owns. The alternative — refusing to create a world, or
// panicking, after 65535 creations — would turn a diagnostic aid into an
// outage for a long-running process that creates a world per match or per
// tick, which is the worse failure.
func nextWorldToken() uint16 {
	return uint16((worldTokenCounter.Add(1) - 1) % worldTokenCount)
}

// World manages all physics entities, dynamic simulation, and queries
// (upstream b2World). Create one with NewWorld.
type World struct {
	arena           arena
	broadPhase      broadPhase
	constraintGraph constraintGraph

	// The body id pool is used to allocate and recycle body ids. Body ids
	// provide a stable identifier for users, but incur cache misses when used
	// to access body data. Aligns with bodies.
	bodyIDPool idPool

	// This is a sparse array that maps body ids to the body data stored in
	// solver sets. As sims move within a set or across sets, indices come
	// from the id pool.
	bodies []body

	// Provides free list for solver sets.
	solverSetIDPool idPool

	// Solver sets allow sims to be stored in contiguous arrays. The first
	// set is all static sims. The second set is active sims. The third set is
	// disabled sims. The remaining sets are sleeping islands.
	solverSets []solverSet

	// Used to create stable ids for joints.
	jointIDPool idPool

	// This is a sparse array that maps joint ids to the joint data stored in
	// the constraint graph or in the solver sets.
	joints []joint

	// Used to create stable ids for contacts.
	contactIDPool idPool

	// This is a sparse array that maps contact ids to the contact data stored
	// in the constraint graph or in the solver sets.
	contacts []contact

	// Used to create stable ids for islands.
	islandIDPool idPool

	// Persistent islands.
	islands []island

	shapeIDPool idPool
	chainIDPool idPool

	// These are sparse arrays that point into the pools above.
	shapes      []shape
	chainShapes []chainShape

	// This is a dense array of sensor data. The per-sensor change bits live
	// in the per-worker taskContext.sensorEventBits (upstream
	// b2SensorTaskContext — see solver.go and sensor.go).
	sensors []sensor

	bodyMoveEvents     []BodyMoveEvent
	sensorBeginEvents  []SensorBeginTouchEvent
	contactBeginEvents []ContactBeginTouchEvent

	// End events are double buffered so that the user doesn't need to flush
	// events.
	sensorEndEvents    [2][]SensorEndTouchEvent
	contactEndEvents   [2][]ContactEndTouchEvent
	endEventArrayIndex int

	contactHitEvents []ContactHitEvent
	jointEvents      []JointEvent

	// Per-worker solver scratch (upstream b2TaskContext array), one slot per
	// worker; len == workerCount. Worker 0 is the Step goroutine and the
	// serial path uses only slot 0 (see solver.go).
	taskContexts []taskContext

	// workerCount is the effective worker count: WorldDef.WorkerCount with 0
	// mapped to 1, computed once in NewWorld (no GOMAXPROCS clamp — see the
	// NewWorld comment). Simulation results are byte-identical for every
	// value.
	workerCount int

	// pool is the internal worker pool; nil when workerCount == 1, in which
	// case every stage dispatch takes its serial branch.
	pool *workerPool

	// Used to track debug draw.
	debugBodySet    bitSet
	debugJointSet   bitSet
	debugContactSet bitSet
	debugIslandSet  bitSet

	// Id that is incremented every time step.
	stepIndex uint64

	// Identify islands for splitting as follows:
	//   - islands are split so smaller islands can sleep
	//   - when a body comes to rest and its sleep timer trips, the island is
	//     flagged for splitting if it has removed constraints
	//   - islands that have removed constraints are split first
	//   - otherwise the awake islands that have bodies wanting to sleep are
	//     the splitting candidates
	//   - if no bodies want to sleep then there is no reason to split islands
	splitIslandID int

	gravity                Vec2
	hitEventThreshold      float64
	restitutionThreshold   float64
	maxLinearSpeed         float64
	contactSpeed           float64
	contactHertz           float64
	contactDampingRatio    float64
	contactRecycleDistance float64

	frictionCallback    FrictionCallback
	restitutionCallback RestitutionCallback

	generation uint16

	profile Profile

	preSolveFcn     PreSolveFcn
	preSolveContext any

	customFilterFcn     CustomFilterFcn
	customFilterContext any

	// User data. Deviation from upstream: the C void* becomes a uint64 so the
	// ECS wrapper can pack an entity id directly.
	userData uint64

	// Remember type step used for reporting forces and torques.
	// Inverse sub-step.
	invH float64

	// Inverse full-step.
	invDt float64

	taskCount int

	worldID uint16

	enableSleep            bool
	locked                 bool
	enableWarmStarting     bool
	enableContactSoftening bool

	// stepPanicked is set when a panic (or runtime.Goexit) unwound Step
	// mid-flight, abandoning a half-integrated simulation state. Step and
	// every world-level read path (queries, event accessors, Draw) check it
	// first via panicIfPoisoned and fail loudly in every build: continuing
	// would silently diverge from any deterministic replica, and the latched
	// locked flag would otherwise turn every later Step into a silent no-op
	// and every read into a silent zero-value wrong answer. Destroy still
	// works on a poisoned world. Go-only state — upstream C has no
	// recoverable unwind out of b2World_Step.
	stepPanicked      bool
	enableContinuous  bool
	enableSpeculative bool
	inUse             bool

	// Single-threaded query scratch: the callback contexts and tree inputs
	// that upstream keeps on the C stack. Go must assume a pointer passed to
	// a callback escapes, so a stack local would heap allocate on every
	// query; see worldQueryScratch in world_query.go.
	//
	// Kept last on purpose. It is large (several hundred bytes) and is touched
	// exactly once per query, so placing it earlier would push the small hot
	// fields below it (worldID, locked, the solver tuning scalars) onto extra
	// cache lines.
	queryScratch worldQueryScratch

	// stepCtx is the per-step context (upstream b2StepContext, a C stack
	// local). It lives on the World because a Go stack local escapes into
	// the stage closures Step builds, heap-allocating every step even on the
	// serial path; Step zeroes and refills it each call, so no state leaks
	// between steps. Kept at the end next to queryScratch for the same
	// cache-line reason (mid-struct scratch cost 12%/step in a previous
	// layout pass).
	stepCtx stepContext
}

// defaultFrictionCallback mirrors b2DefaultFrictionCallback.
func defaultFrictionCallback(frictionA float64, materialA uint64, frictionB float64, materialB uint64) float64 {
	_ = materialA
	_ = materialB
	return math.Sqrt(frictionA * frictionB)
}

// defaultRestitutionCallback mirrors b2DefaultRestitutionCallback.
func defaultRestitutionCallback(restitutionA float64, materialA uint64, restitutionB float64, materialB uint64) float64 {
	_ = materialA
	_ = materialB
	return maxFloat(restitutionA, restitutionB)
}

// NewWorld creates a world for rigid body simulation. A world contains
// bodies, shapes, and constraints. Deviation from upstream b2CreateWorld:
// this returns a *World instead of a b2WorldId into a global registry (see
// the file header).
func NewWorld(def *WorldDef) *World {
	requireInitialized(def.initialized, "WorldDef", "DefaultWorldDef")
	requireValidDefField(def.WorkerCount >= 0 && def.WorkerCount <= MaxWorkers,
		"WorldDef", "WorkerCount", "must be in [0, MaxWorkers]; 0 means 1 (serial)")

	initializeContactRegisters()

	w := &World{}

	w.worldID = nextWorldToken()
	w.generation = 0
	w.inUse = true

	w.arena = createArena()
	createBroadPhase(&w.broadPhase)
	createGraph(&w.constraintGraph, 16)

	// pools
	w.bodyIDPool = createIDPool()
	w.bodies = make([]body, 0, 16)
	w.solverSets = make([]solverSet, 0, 8)

	// add empty static, active, and disabled body sets
	w.solverSetIDPool = createIDPool()
	var set solverSet

	// static set
	set.setIndex = allocID(&w.solverSetIDPool)
	w.solverSets = append(w.solverSets, set)
	assert(w.solverSets[staticSet].setIndex == staticSet)

	// disabled set
	set.setIndex = allocID(&w.solverSetIDPool)
	w.solverSets = append(w.solverSets, set)
	assert(w.solverSets[disabledSet].setIndex == disabledSet)

	// awake set
	set.setIndex = allocID(&w.solverSetIDPool)
	w.solverSets = append(w.solverSets, set)
	assert(w.solverSets[awakeSet].setIndex == awakeSet)

	w.shapeIDPool = createIDPool()
	w.shapes = make([]shape, 0, 16)

	w.chainIDPool = createIDPool()
	w.chainShapes = make([]chainShape, 0, 4)

	w.contactIDPool = createIDPool()
	w.contacts = make([]contact, 0, 16)

	w.jointIDPool = createIDPool()
	w.joints = make([]joint, 0, 16)

	w.islandIDPool = createIDPool()
	w.islands = make([]island, 0, 8)

	w.sensors = make([]sensor, 0, 4)

	w.bodyMoveEvents = make([]BodyMoveEvent, 0, 4)
	w.sensorBeginEvents = make([]SensorBeginTouchEvent, 0, 4)
	w.sensorEndEvents[0] = make([]SensorEndTouchEvent, 0, 4)
	w.sensorEndEvents[1] = make([]SensorEndTouchEvent, 0, 4)
	w.contactBeginEvents = make([]ContactBeginTouchEvent, 0, 4)
	w.contactEndEvents[0] = make([]ContactEndTouchEvent, 0, 4)
	w.contactEndEvents[1] = make([]ContactEndTouchEvent, 0, 4)
	w.contactHitEvents = make([]ContactHitEvent, 0, 4)
	w.jointEvents = make([]JointEvent, 0, 4)
	w.endEventArrayIndex = 0

	w.stepIndex = 0
	w.splitIslandID = NullIndex
	w.taskCount = 0
	w.gravity = def.Gravity
	w.hitEventThreshold = def.HitEventThreshold
	w.restitutionThreshold = def.RestitutionThreshold
	w.maxLinearSpeed = def.MaximumLinearSpeed
	w.contactSpeed = def.ContactSpeed
	w.contactHertz = def.ContactHertz
	w.contactDampingRatio = def.ContactDampingRatio
	w.contactRecycleDistance = ContactRecycleDistance

	if def.FrictionCallback == nil {
		w.frictionCallback = defaultFrictionCallback
	} else {
		w.frictionCallback = def.FrictionCallback
	}

	if def.RestitutionCallback == nil {
		w.restitutionCallback = defaultRestitutionCallback
	} else {
		w.restitutionCallback = def.RestitutionCallback
	}

	w.enableSleep = def.EnableSleep
	w.locked = false
	w.enableWarmStarting = true
	w.enableContactSoftening = def.EnableContactSoftening
	w.enableContinuous = def.EnableContinuous
	w.enableSpeculative = true
	w.userData = def.UserData

	// Effective worker count: WorkerCount taken as given (0 means 1), already
	// validated <= MaxWorkers above. Deliberately NOT clamped to
	// runtime.GOMAXPROCS(0): upstream does not clamp either — the worker
	// count is the caller's explicit choice; oversubscribing cores with
	// goroutines is legal Go; and a clamp made every worker-matrix test and
	// CI row silently degrade to GOMAXPROCS-way partitions (vacuously green
	// on small runners). Results are byte-identical for every value by
	// design, so an oversubscribed count costs throughput only.
	w.workerCount = max(def.WorkerCount, 1)

	w.taskContexts = make([]taskContext, w.workerCount)
	for i := range w.taskContexts {
		w.taskContexts[i] = createTaskContext()
	}

	if w.workerCount > 1 {
		w.pool = newWorkerPool(w.workerCount)
	}

	w.debugBodySet = createBitSet(256)
	w.debugJointSet = createBitSet(256)
	w.debugContactSet = createBitSet(256)
	w.debugIslandSet = createBitSet(256)

	return w
}

// Destroy destroys the world and all its contents
// (upstream b2DestroyWorld). Ids created from this world become invalid.
func (w *World) Destroy() {
	// Tear the worker pool down first — the goroutines must be gone before
	// the *w = World{} wipe below invalidates the state they reference.
	if w.pool != nil {
		w.pool.close()
		w.pool = nil
	}

	for i := range w.taskContexts {
		destroyTaskContext(&w.taskContexts[i])
	}
	w.taskContexts = nil
	destroyBitSet(&w.debugBodySet)
	destroyBitSet(&w.debugJointSet)
	destroyBitSet(&w.debugContactSet)
	destroyBitSet(&w.debugIslandSet)

	w.bodyMoveEvents = nil
	w.sensorBeginEvents = nil
	w.sensorEndEvents[0] = nil
	w.sensorEndEvents[1] = nil
	w.contactBeginEvents = nil
	w.contactEndEvents[0] = nil
	w.contactEndEvents[1] = nil
	w.contactHitEvents = nil
	w.jointEvents = nil

	chainCapacity := len(w.chainShapes)
	for i := range chainCapacity {
		chain := &w.chainShapes[i]
		if chain.id != NullIndex {
			freeChainData(chain)
		} else {
			assert(chain.shapeIndices == nil)
			assert(chain.materials == nil)
		}
	}

	sensorCount := len(w.sensors)
	for i := range sensorCount {
		w.sensors[i].hits = nil
		w.sensors[i].overlaps1 = nil
		w.sensors[i].overlaps2 = nil
	}

	w.sensors = nil

	w.bodies = nil
	w.shapes = nil
	w.chainShapes = nil
	w.contacts = nil
	w.joints = nil

	for i := range w.islands {
		w.islands[i].bodies = nil
		w.islands[i].contacts = nil
		w.islands[i].joints = nil
	}
	w.islands = nil

	// Destroy solver sets
	setCapacity := len(w.solverSets)
	for i := range setCapacity {
		set := &w.solverSets[i]
		if set.setIndex != NullIndex {
			w.destroySolverSet(i)
		}
	}

	w.solverSets = nil

	destroyGraph(&w.constraintGraph)
	destroyBroadPhase(&w.broadPhase)

	destroyIDPool(&w.bodyIDPool)
	destroyIDPool(&w.shapeIDPool)
	destroyIDPool(&w.chainIDPool)
	destroyIDPool(&w.contactIDPool)
	destroyIDPool(&w.jointIDPool)
	destroyIDPool(&w.islandIDPool)
	destroyIDPool(&w.solverSetIDPool)

	destroyArena(&w.arena)

	// Wipe world but preserve the owner token and generation, matching
	// upstream b2DestroyWorld which restores world->worldId (its registry
	// slot) and bumps the generation.
	generation := w.generation
	worldID := w.worldID
	*w = World{}
	w.worldID = worldID
	w.generation = generation + 1
}

// ID returns the WorldID of this world. This replaces the b2WorldId returned
// by upstream b2CreateWorld (see the file header for the registry deviation).
func (w *World) ID() WorldID {
	return WorldID{index1: w.worldID + 1, generation: w.generation}
}

// IsWorldValid reports whether a world id references this world and is not
// stale (upstream b2World_IsValid; see the file header for the registry
// deviation).
func (w *World) IsWorldValid(id WorldID) bool {
	if id.index1 < 1 {
		return false
	}

	// Upstream also bounds index1 by B2_MAX_WORLDS to stay inside the registry
	// array. There is no array here, and the owner token comparison below is
	// strictly stronger, so the bound is dropped.
	if w.worldID != id.index1-1 {
		// id was minted by a different world
		return false
	}

	if !w.inUse {
		return false
	}

	return id.generation == w.generation
}

// ownsToken reports whether an id's world0 field carries this world's owner
// token. Upstream has no equivalent because it resolves the world *from*
// id.world0 (b2GetWorldFromId), which makes applying one world's id to another
// impossible by construction; here the world is the receiver, so it has to
// reject foreign ids itself. See the file header.
func (w *World) ownsToken(world0 uint16) bool {
	return world0 == w.worldID
}

// stepPanickedMsg names both the cause and the remedy; every poisoned-world
// panic uses it so a recovered tick fails the same way no matter which entry
// point it hits first.
const stepPanickedMsg = "box2d: this world's previous Step was unwound by a panic; " +
	"the simulation state is incomplete — destroy this world and rebuild it"

// panicIfPoisoned fails loudly on a world whose Step was unwound by a panic
// (see the stepPanicked field). It runs BEFORE the assert(!w.locked)
// reentrancy guards so the box2d_asserts build reports this message instead
// of a bare assertion failure, and it is always on (require* tier): the
// latched lock would otherwise turn reads into silent zero-value wrong
// answers. On the genuinely reentrant path (a query from inside a step
// callback) stepPanicked is false, so behavior there is unchanged.
//
// Scope — a deliberate contract call: Step and every READ path (the world
// queries, event accessors, Draw, and the per-body/shape/chain data getters)
// check the poison, because a zero-value read is a wrong answer a caller
// acts on. The MUTATORS keep their plain locked-guard shape unchecked: a
// recovered tick cannot complete without hitting a read or Step, so it dies
// loudly before its writes can matter, and threading the check through the
// ~30 locked-guarded setters would buy no additional safety for the churn.
func (w *World) panicIfPoisoned() {
	if w.stepPanicked {
		panic(stepPanickedMsg)
	}
}

// IsBodyValid reports whether a body id is valid in this world. Can be used
// to detect orphaned ids. Provides validation for up to 64K allocations
// (upstream b2Body_IsValid).
func (w *World) IsBodyValid(id BodyID) bool {
	if w.worldID != id.world0 || !w.inUse {
		// id was minted by a different world, or this world is destroyed
		return false
	}

	if id.index1 < 1 || len(w.bodies) < int(id.index1) {
		// invalid index
		return false
	}

	b := &w.bodies[id.index1-1]
	if b.setIndex == NullIndex {
		// this was freed
		return false
	}

	assert(b.localIndex != NullIndex)

	if b.generation != id.generation {
		// this id is orphaned
		return false
	}

	return true
}

// IsShapeValid reports whether a shape id is valid in this world
// (upstream b2Shape_IsValid).
func (w *World) IsShapeValid(id ShapeID) bool {
	if w.worldID != id.world0 || !w.inUse {
		// id was minted by a different world, or this world is destroyed
		return false
	}

	shapeID := int(id.index1) - 1
	if shapeID < 0 || len(w.shapes) <= shapeID {
		return false
	}

	s := &w.shapes[shapeID]
	if s.id == NullIndex {
		// shape is free
		return false
	}

	assert(s.id == shapeID)

	return id.generation == s.generation
}

// IsChainValid reports whether a chain id is valid in this world
// (upstream b2Chain_IsValid).
func (w *World) IsChainValid(id ChainID) bool {
	if w.worldID != id.world0 || !w.inUse {
		// id was minted by a different world, or this world is destroyed
		return false
	}

	chainID := int(id.index1) - 1
	if chainID < 0 || len(w.chainShapes) <= chainID {
		return false
	}

	chain := &w.chainShapes[chainID]
	if chain.id == NullIndex {
		// chain is free
		return false
	}

	assert(chain.id == chainID)

	return id.generation == chain.generation
}

// IsJointValid reports whether a joint id is valid in this world
// (upstream b2Joint_IsValid).
func (w *World) IsJointValid(id JointID) bool {
	if w.worldID != id.world0 || !w.inUse {
		// id was minted by a different world, or this world is destroyed
		return false
	}

	jointID := int(id.index1) - 1
	if jointID < 0 || len(w.joints) <= jointID {
		return false
	}

	j := &w.joints[jointID]
	if j.jointID == NullIndex {
		// joint is free
		return false
	}

	assert(j.jointID == jointID)

	return id.generation == j.generation
}

// IsContactValid reports whether a contact id is valid in this world
// (upstream b2Contact_IsValid).
func (w *World) IsContactValid(id ContactID) bool {
	if w.worldID != id.world0 || !w.inUse {
		// id was minted by a different world, or this world is destroyed
		return false
	}

	contactID := int(id.index1) - 1
	if contactID < 0 || len(w.contacts) <= contactID {
		return false
	}

	c := &w.contacts[contactID]
	if c.contactID == NullIndex {
		// contact is free
		return false
	}

	assert(c.contactID == contactID)

	return id.generation == c.generation
}

// EnableSleeping enables/disables sleep. If your application does not need
// sleeping, you can gain some performance by disabling sleep completely at
// the world level (upstream b2World_EnableSleeping).
func (w *World) EnableSleeping(flag bool) {
	assert(!w.locked)
	if w.locked {
		return
	}

	if flag == w.enableSleep {
		return
	}

	w.enableSleep = flag

	if !flag {
		setCount := len(w.solverSets)
		for i := firstSleepingSet; i < setCount; i++ {
			set := &w.solverSets[i]
			if len(set.bodySims) > 0 {
				w.wakeSolverSet(i)
			}
		}
	}
}

// IsSleepingEnabled reports whether body sleeping is enabled
// (upstream b2World_IsSleepingEnabled).
func (w *World) IsSleepingEnabled() bool {
	return w.enableSleep
}

// EnableWarmStarting enables/disables constraint warm starting. Advanced
// feature for testing. Disabling warm starting greatly reduces stability and
// provides no performance gain (upstream b2World_EnableWarmStarting).
func (w *World) EnableWarmStarting(flag bool) {
	assert(!w.locked)
	if w.locked {
		return
	}

	w.enableWarmStarting = flag
}

// IsWarmStartingEnabled reports whether constraint warm starting is enabled
// (upstream b2World_IsWarmStartingEnabled).
func (w *World) IsWarmStartingEnabled() bool {
	return w.enableWarmStarting
}

// AwakeBodyCount returns the number of awake bodies
// (upstream b2World_GetAwakeBodyCount).
func (w *World) AwakeBodyCount() int {
	awake := &w.solverSets[awakeSet]
	return len(awake.bodySims)
}

// EnableContinuous enables/disables continuous collision between dynamic and
// static bodies. Generally you should keep continuous collision enabled to
// prevent fast moving objects from going through static objects. The
// performance gain from disabling continuous collision is minor
// (upstream b2World_EnableContinuous).
func (w *World) EnableContinuous(flag bool) {
	assert(!w.locked)
	if w.locked {
		return
	}

	w.enableContinuous = flag
}

// IsContinuousEnabled reports whether continuous collision is enabled
// (upstream b2World_IsContinuousEnabled).
func (w *World) IsContinuousEnabled() bool {
	return w.enableContinuous
}

// SetRestitutionThreshold adjusts the restitution threshold, usually in
// meters per second (upstream b2World_SetRestitutionThreshold).
func (w *World) SetRestitutionThreshold(value float64) {
	assert(!w.locked)
	if w.locked {
		return
	}

	w.restitutionThreshold = clampFloat(value, 0.0, math.MaxFloat64)
}

// RestitutionThreshold returns the restitution speed threshold, usually in
// meters per second (upstream b2World_GetRestitutionThreshold).
func (w *World) RestitutionThreshold() float64 {
	return w.restitutionThreshold
}

// SetHitEventThreshold adjusts the hit event threshold, usually in meters per
// second (upstream b2World_SetHitEventThreshold).
func (w *World) SetHitEventThreshold(value float64) {
	assert(!w.locked)
	if w.locked {
		return
	}

	w.hitEventThreshold = clampFloat(value, 0.0, math.MaxFloat64)
}

// HitEventThreshold returns the hit event speed threshold, usually in meters
// per second (upstream b2World_GetHitEventThreshold).
func (w *World) HitEventThreshold() float64 {
	return w.hitEventThreshold
}

// SetContactTuning adjusts contact tuning parameters: hertz is the contact
// stiffness (cycles per second), dampingRatio the contact bounciness with 1
// being critical damping (non-dimensional), and pushSpeed the maximum contact
// constraint push out speed (meters per second)
// (upstream b2World_SetContactTuning).
//
// Note: advanced feature.
func (w *World) SetContactTuning(hertz, dampingRatio, pushSpeed float64) {
	assert(!w.locked)
	if w.locked {
		return
	}

	w.contactHertz = clampFloat(hertz, 0.0, math.MaxFloat64)
	w.contactDampingRatio = clampFloat(dampingRatio, 0.0, math.MaxFloat64)
	w.contactSpeed = clampFloat(pushSpeed, 0.0, math.MaxFloat64)
}

// SetContactRecycleDistance sets the contact recycle distance
// (upstream b2World_SetContactRecycleDistance).
func (w *World) SetContactRecycleDistance(recycleDistance float64) {
	assert(!w.locked)
	if w.locked {
		return
	}

	w.contactRecycleDistance = clampFloat(recycleDistance, 0.0, math.MaxFloat64)
}

// ContactRecycleDistance returns the contact recycle distance
// (upstream b2World_GetContactRecycleDistance).
func (w *World) ContactRecycleDistance() float64 {
	return w.contactRecycleDistance
}

// SetMaximumLinearSpeed sets the maximum linear speed. Usually in meters per
// second (upstream b2World_SetMaximumLinearSpeed).
func (w *World) SetMaximumLinearSpeed(maximumLinearSpeed float64) {
	assert(IsValidFloat(maximumLinearSpeed) && maximumLinearSpeed > 0.0)

	assert(!w.locked)
	if w.locked {
		return
	}

	w.maxLinearSpeed = maximumLinearSpeed
}

// MaximumLinearSpeed returns the maximum linear speed. Usually in meters per
// second (upstream b2World_GetMaximumLinearSpeed).
func (w *World) MaximumLinearSpeed() float64 {
	return w.maxLinearSpeed
}

// Profile returns the current world performance profile
// (upstream b2World_GetProfile).
func (w *World) Profile() Profile {
	return w.profile
}

// Counters returns world counters and sizes (upstream b2World_GetCounters).
func (w *World) Counters() Counters {
	var s Counters
	s.BodyCount = getIDCount(&w.bodyIDPool)
	s.ShapeCount = getIDCount(&w.shapeIDPool)
	s.ContactCount = getIDCount(&w.contactIDPool)
	s.JointCount = getIDCount(&w.jointIDPool)
	s.IslandCount = getIDCount(&w.islandIDPool)

	staticTree := &w.broadPhase.trees[StaticBody]
	s.StaticTreeHeight = staticTree.GetHeight()

	dynamicTree := &w.broadPhase.trees[DynamicBody]
	kinematicTree := &w.broadPhase.trees[KinematicBody]
	s.TreeHeight = maxInt(dynamicTree.GetHeight(), kinematicTree.GetHeight())

	s.StackUsed = getMaxArenaAllocation(&w.arena)
	s.ByteCount = 0 // upstream b2GetByteCount tracks global allocations; no Go counterpart
	s.TaskCount = w.taskCount

	for i := range GraphColorCount {
		s.ColorCounts[i] = len(w.constraintGraph.colors[i].contactSims) + len(w.constraintGraph.colors[i].jointSims)
	}
	return s
}

// SetUserData sets the user data on the world (upstream b2World_SetUserData).
func (w *World) SetUserData(userData uint64) {
	w.userData = userData
}

// UserData returns the user data stored on the world
// (upstream b2World_GetUserData).
func (w *World) UserData() uint64 {
	return w.userData
}

// SetFrictionCallback sets the friction callback. Passing nil restores the
// default mixing rule sqrt(frictionA * frictionB)
// (upstream b2World_SetFrictionCallback).
func (w *World) SetFrictionCallback(callback FrictionCallback) {
	assert(!w.locked)
	if w.locked {
		return
	}

	if callback != nil {
		w.frictionCallback = callback
	} else {
		w.frictionCallback = defaultFrictionCallback
	}
}

// SetRestitutionCallback sets the restitution callback. Passing nil restores
// the default mixing rule max(restitutionA, restitutionB)
// (upstream b2World_SetRestitutionCallback).
func (w *World) SetRestitutionCallback(callback RestitutionCallback) {
	assert(!w.locked)
	if w.locked {
		return
	}

	if callback != nil {
		w.restitutionCallback = callback
	} else {
		w.restitutionCallback = defaultRestitutionCallback
	}
}

// SetCustomFilterCallback sets the custom filter callback. This is optional
// (upstream b2World_SetCustomFilterCallback).
func (w *World) SetCustomFilterCallback(fcn CustomFilterFcn, ctx any) {
	w.customFilterFcn = fcn
	w.customFilterContext = ctx
}

// SetPreSolveCallback sets the pre-solve callback. This is optional
// (upstream b2World_SetPreSolveCallback).
func (w *World) SetPreSolveCallback(fcn PreSolveFcn, ctx any) {
	w.preSolveFcn = fcn
	w.preSolveContext = ctx
}

// SetGravity sets the gravity vector for the entire world. Box2D has no
// up-vector. This is usually in m/s^2 (upstream b2World_SetGravity).
//
// Note: this does not wake sleeping bodies.
func (w *World) SetGravity(gravity Vec2) {
	w.gravity = gravity
}

// Gravity returns the gravity vector (upstream b2World_GetGravity).
func (w *World) Gravity() Vec2 {
	return w.gravity
}

// RebuildStaticTree rebuilds the static broad-phase tree. This is a slow
// operation used when many static shapes have been created or modified
// (upstream b2World_RebuildStaticTree).
func (w *World) RebuildStaticTree() {
	assert(!w.locked)
	if w.locked {
		return
	}

	staticTree := &w.broadPhase.trees[StaticBody]
	staticTree.Rebuild(true)
}

// EnableSpeculative enables/disables speculative contacts. Advanced feature
// for testing (upstream b2World_EnableSpeculative).
func (w *World) EnableSpeculative(flag bool) {
	w.enableSpeculative = flag
}

// validateConnectivity mirrors b2ValidateConnectivity. Validation is compiled
// out in this port, matching upstream release builds (B2_ENABLE_VALIDATION
// off). TODO(E7): port the full validator behind debugAsserts.
func (w *World) validateConnectivity() {
}

// validateSolverSets mirrors b2ValidateSolverSets. Validation is compiled out
// in this port, matching upstream release builds (B2_ENABLE_VALIDATION off).
// TODO(E7): port the full validator behind debugAsserts.
func (w *World) validateSolverSets() {
}

// validateContacts mirrors b2ValidateContacts. Validation is compiled out in
// this port, matching upstream release builds (B2_ENABLE_VALIDATION off).
// TODO(E7): port the full validator behind debugAsserts.
func (w *World) validateContacts() {
}
