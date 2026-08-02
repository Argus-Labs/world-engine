// Ported to Go from Box2D v3.2.0 (https://github.com/erincatto/box2d) — file src/arena_allocator.c, src/arena_allocator.h.
//
// PRAGMATIC PORT: upstream b2ArenaAllocator is a stack-like allocator that
// hands out raw byte blocks (b2AllocateArenaItem/b2FreeArenaItem) with strict
// LIFO nesting, falling back to the heap when the arena is exhausted. Go
// forbids reinterpreting raw bytes without unsafe, so this port gives each
// upstream b2AllocateArenaItem call site a dedicated reusable typed scratch
// slice on this struct instead. Allocate/free bookkeeping is kept so
// b2GetArenaAllocation/b2GetMaxArenaAllocation (Counters.StackUsed) have
// meaningful parity.
//
// Deviations from upstream:
//   - Allocation sizes are tracked in ELEMENTS, not bytes. Upstream tracks
//     bytes (with 32-byte alignment); byte counts are meaningless across the
//     float32→float64 change, so Counters.StackUsed reports element counts.
//   - b2GrowArena has no counterpart: the typed scratch slices grow in place
//     via ensureCapacity semantics on each allocation.
//   - The b2ArenaEntry bookkeeping array is replaced by a per-slice
//     outstanding-count field; the LIFO nesting assert collapses to
//     one-outstanding-allocation-per-slice.
//
// Call sites: E5 added "mass data" (b2UpdateBodyMassData). E7 adds the
// narrow-phase contact pointer array ("contacts" in b2Collide), the scalar
// contact constraints ("contact constraint" + "overflow contact constraint"
// merged, see solver.go), the bullet body index array ("bullet bodies") and
// the splitIsland int scratch arrays (island.c).

package box2d

// arenaIntSlot is one reusable []int scratch slot (one per upstream
// b2AllocateArenaItem call site that hands out an int array).
type arenaIntSlot struct {
	buf   []int
	count int
}

// arena is the per-World scratch arena (upstream b2ArenaAllocator).
type arena struct {
	// allocation is the current outstanding allocation, in elements.
	allocation int

	// maxAllocation is the high-water mark of allocation, in elements.
	maxAllocation int

	// massData services the "mass data" call site in updateBodyMassData
	// (upstream b2AllocateArenaItem(&world->arena, shapeCount *
	// sizeof(b2MassData), "mass data")).
	massData      []MassData
	massDataCount int

	// contactPtrs services the "contacts" call site in collide (upstream
	// b2Collide gathering all awake contact sim pointers).
	contactPtrs      []*contactSim
	contactPtrsCount int

	// contactConstraints services the scalar contact constraint scratch in
	// solve. Upstream has two call sites ("contact constraint" for the SIMD
	// colors and "overflow contact constraint"); this all-scalar port merges
	// them into one flat array partitioned per color (see solver.go).
	contactConstraints      []contactConstraint
	contactConstraintsCount int

	// bulletBodies services the "bullet bodies" call site in solve.
	bulletBodies      []int
	bulletBodiesCount int

	// splitIsland scratch (island.c b2SplitIsland call sites, same names).
	splitParents                arenaIntSlot // "parents"
	splitContactCounts          arenaIntSlot // "contact counts"
	splitJointCounts            arenaIntSlot // "joint counts"
	splitRanks                  arenaIntSlot // "ranks"
	splitRootMap                arenaIntSlot // "root map"
	splitComponentBodyCounts    arenaIntSlot // "component body counts"
	splitComponentContactCounts arenaIntSlot // "component contact counts"
	splitComponentJointCounts   arenaIntSlot // "component joint counts"
	splitIslandIDs              arenaIntSlot // "island ids"
}

// createArena mirrors b2CreateArenaAllocator. The upstream byte capacity hint
// has no meaning for typed scratch slices, so it is dropped.
func createArena() arena {
	return arena{}
}

// destroyArena mirrors b2DestroyArenaAllocator.
func destroyArena(a *arena) {
	assert(a.allocation == 0)
	*a = arena{}
}

// allocMassData returns a scratch []MassData of the requested length
// (upstream b2AllocateArenaItem, "mass data"). The contents are zeroed.
func (a *arena) allocMassData(count int) []MassData {
	assert(a.massDataCount == 0)
	a.massDataCount = count
	a.allocation += count
	if a.allocation > a.maxAllocation {
		a.maxAllocation = a.allocation
	}

	if cap(a.massData) < count {
		a.massData = make([]MassData, count)
	}

	s := a.massData[:count]
	for i := range s {
		s[i] = MassData{}
	}

	return s
}

// freeMassData releases the scratch obtained from allocMassData (upstream
// b2FreeArenaItem).
func (a *arena) freeMassData() {
	a.allocation -= a.massDataCount
	a.massDataCount = 0
	assert(a.allocation >= 0)
}

// bumpAllocation records count outstanding elements.
func (a *arena) bumpAllocation(count int) {
	a.allocation += count
	if a.allocation > a.maxAllocation {
		a.maxAllocation = a.allocation
	}
}

// allocContactPtrs returns a zeroed scratch []*contactSim of the requested
// length (upstream b2AllocateArenaItem, "contacts").
func (a *arena) allocContactPtrs(count int) []*contactSim {
	assert(a.contactPtrsCount == 0)
	a.contactPtrsCount = count
	a.bumpAllocation(count)

	if cap(a.contactPtrs) < count {
		a.contactPtrs = make([]*contactSim, count)
	}

	s := a.contactPtrs[:count]
	for i := range s {
		s[i] = nil
	}

	return s
}

// freeContactPtrs releases the scratch obtained from allocContactPtrs
// (upstream b2FreeArenaItem). The pointers are cleared so the arena does not
// pin contact sims across steps.
func (a *arena) freeContactPtrs() {
	s := a.contactPtrs[:a.contactPtrsCount]
	for i := range s {
		s[i] = nil
	}

	a.allocation -= a.contactPtrsCount
	a.contactPtrsCount = 0
	assert(a.allocation >= 0)
}

// allocContactConstraints returns a zeroed scratch []contactConstraint of the
// requested length (upstream b2AllocateArenaItem, "contact constraint" and
// "overflow contact constraint" merged — see the file header).
func (a *arena) allocContactConstraints(count int) []contactConstraint {
	assert(a.contactConstraintsCount == 0)
	a.contactConstraintsCount = count
	a.bumpAllocation(count)

	if cap(a.contactConstraints) < count {
		a.contactConstraints = make([]contactConstraint, count)
	}

	s := a.contactConstraints[:count]
	for i := range s {
		s[i] = contactConstraint{}
	}

	return s
}

// freeContactConstraints releases the scratch obtained from
// allocContactConstraints (upstream b2FreeArenaItem).
func (a *arena) freeContactConstraints() {
	a.allocation -= a.contactConstraintsCount
	a.contactConstraintsCount = 0
	assert(a.allocation >= 0)
}

// allocBulletBodies returns a zeroed scratch []int of the requested length
// (upstream b2AllocateArenaItem, "bullet bodies").
func (a *arena) allocBulletBodies(count int) []int {
	assert(a.bulletBodiesCount == 0)
	a.bulletBodiesCount = count
	a.bumpAllocation(count)

	if cap(a.bulletBodies) < count {
		a.bulletBodies = make([]int, count)
	}

	s := a.bulletBodies[:count]
	for i := range s {
		s[i] = 0
	}

	return s
}

// freeBulletBodies releases the scratch obtained from allocBulletBodies
// (upstream b2FreeArenaItem).
func (a *arena) freeBulletBodies() {
	a.allocation -= a.bulletBodiesCount
	a.bulletBodiesCount = 0
	assert(a.allocation >= 0)
}

// allocInts returns a zeroed scratch []int of the requested length from the
// given slot (upstream b2AllocateArenaItem for one int-array call site).
func (a *arena) allocInts(slot *arenaIntSlot, count int) []int {
	assert(slot.count == 0)
	slot.count = count
	a.bumpAllocation(count)

	if cap(slot.buf) < count {
		slot.buf = make([]int, count)
	}

	s := slot.buf[:count]
	for i := range s {
		s[i] = 0
	}

	return s
}

// freeInts releases the scratch obtained from allocInts (upstream
// b2FreeArenaItem).
func (a *arena) freeInts(slot *arenaIntSlot) {
	a.allocation -= slot.count
	slot.count = 0
	assert(a.allocation >= 0)
}

// getArenaCapacity mirrors b2GetArenaCapacity, in elements.
func getArenaCapacity(a *arena) int {
	return cap(a.massData)
}

// getArenaAllocation mirrors b2GetArenaAllocation, in elements.
func getArenaAllocation(a *arena) int {
	return a.allocation
}

// getMaxArenaAllocation mirrors b2GetMaxArenaAllocation, in elements.
func getMaxArenaAllocation(a *arena) int {
	return a.maxAllocation
}
