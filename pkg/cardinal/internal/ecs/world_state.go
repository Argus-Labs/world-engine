package ecs

import (
	"fmt"
	"math"
	"reflect"
	"sync"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// EntityID identifies an entity. The low 32 bits are its index in the entity table, and the high 32
// bits are that index's generation when the entity was created. Destroying an entity advances its
// index's generation, so the ID stops resolving even after a new entity reuses the index. Generations
// start at 1, so the zero ID never refers to an entity.
type EntityID uint64

// newEntityID packs an index and a generation into an EntityID.
func newEntityID(index, generation uint32) EntityID {
	return EntityID(generation)<<32 | EntityID(index)
}

// index returns the entity's index in the entity table. A destroyed entity's index is reused by a
// later one.
func (e EntityID) index() uint32 { return uint32(e) } //nolint:gosec // the low 32 bits are the index

// generation returns how many times the entity's index had been reused, plus one, when it was created.
func (e EntityID) generation() uint32 { return uint32(e >> 32) }

// String formats the ID as its index and generation, such as 7v1 for index 7 at generation 1.
func (e EntityID) String() string { return fmt.Sprintf("%dv%d", e.index(), e.generation()) }

// maxEntityIndex is the largest index an entity can have.
const maxEntityIndex = math.MaxUint32 - 1

const (
	// firstGeneration is the generation of an index's first entity. Zero is never a live generation.
	firstGeneration = 1
	// retiredGeneration marks an index whose generation wrapped. It is never reused, so an old ID
	// can never come back.
	retiredGeneration = 0
)

// noArchetype is the entityRecord archetype of an index that holds no entity.
const noArchetype = -1

// entityRecord is the entity table entry for one index.
type entityRecord struct {
	archetype  int32  // Archetype of the entity at this index, or noArchetype
	generation uint32 // Generation of the entity at this index, or of the next one created here
}

// voidArchetype is an archetype without components.
const voidArchetypeID = 0

// worldState holds the state of the world.
type worldState struct {
	components componentManager // Component type manager
	entityArch []entityRecord   // Entity table: archetype and generation of every index ever used
	free       []uint32         // Free indices to reuse
	archetypes []*archetype     // Array of archetypes
	mu         sync.Mutex
}

// newWorldState creates a new world state.
func newWorldState() *worldState {
	ws := worldState{
		components: newComponentManager(),
		entityArch: make([]entityRecord, 0, sparseCapacity),
		free:       make([]uint32, 0),
		archetypes: make([]*archetype, 1),
	}

	// Insert the void archetype.
	ws.archetypes[voidArchetypeID] = ws.newArchetype(voidArchetypeID, bitmap.Bitmap{})

	return &ws
}

// reset clears all entity data while preserving registered components. Generations restart too, so
// the world hands out the same IDs as a new one.
func (ws *worldState) reset() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.invalidateArchetypes()

	ws.entityArch = ws.entityArch[:0]
	ws.free = ws.free[:0]
	ws.archetypes = ws.archetypes[:1] // Keep the void archetype slot
	// Reset the void archetype to avoid stale data.
	ws.archetypes[voidArchetypeID] = ws.newArchetype(voidArchetypeID, bitmap.Bitmap{})
}

// invalidateArchetypes marks every current archetype changed before they are replaced, so a search
// in progress revalidates the IDs it copied from them against the new state instead of yielding
// dead entities. Reset and a committed restore both call it.
func (ws *worldState) invalidateArchetypes() {
	for _, arch := range ws.archetypes {
		arch.version++
	}
}

// -------------------------------------------------------------------------------------------------
// Entity operations
// -------------------------------------------------------------------------------------------------

// archetypeOf returns the archetype of a live entity. It returns false for a destroyed entity and
// for an ID whose generation is not its index's current one.
func (ws *worldState) archetypeOf(eid EntityID) (archetypeID, bool) {
	index := eid.index()
	if int(index) >= len(ws.entityArch) {
		return 0, false
	}

	record := ws.entityArch[index]
	if record.archetype == noArchetype || record.generation != eid.generation() {
		return 0, false
	}

	return archetypeID(record.archetype), true
}

// setArchetype records the archetype of the entity at index.
func (ws *worldState) setArchetype(index uint32, aid archetypeID) {
	assert.That(aid >= 0 && aid <= math.MaxInt32, "archetype ID out of range")
	ws.entityArch[index].archetype = int32(aid) //nolint:gosec // asserted above
}

// newEntity creates a new entity of the void archetype in the world state. Returns the entity ID.
func (ws *worldState) newEntity() EntityID {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	var index uint32
	if len(ws.free) > 0 { // Reuse the smallest free index (see pushFree)
		index = ws.popFree()
	} else { // Else take the next index
		assert.That(len(ws.entityArch) <= maxEntityIndex, "max number of entities exceeded")
		index = uint32(len(ws.entityArch)) //nolint:gosec // bounded by maxEntityIndex

		if len(ws.entityArch) == cap(ws.entityArch) { // Grow by doubling, as main did
			grown := make([]entityRecord, len(ws.entityArch), max(2*cap(ws.entityArch), 1))
			copy(grown, ws.entityArch)
			ws.entityArch = grown
		}

		ws.entityArch = append(ws.entityArch, entityRecord{archetype: noArchetype, generation: firstGeneration})
	}

	eid := newEntityID(index, ws.entityArch[index].generation)

	// New entities are assigned to the void archetype, which doesn't contain any components.
	voidArchetype := ws.archetypes[voidArchetypeID]
	// Add the entity to the void archetype.
	voidArchetype.newEntity(eid)

	// Update the entity archetype mapping.
	ws.entityArch[index].archetype = voidArchetypeID

	return eid
}

// newEntityWithArchetype creates a new entity of an archetype with the specified components.
// Returns the entity ID. Prefer this method over newEntity + multiple sets because that does a lot
// of moveEntity, which is the most expensive world state operation.
func (ws *worldState) newEntityWithArchetype(components bitmap.Bitmap) EntityID {
	eid := ws.newEntity()
	ws.moveEntity(eid, components)

	return eid
}

// removeEntity removes an entity from the world state. Returns true if the entity is removed, false
// if the entity doesn't exist.
func (ws *worldState) removeEntity(eid EntityID) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	aid, exists := ws.archetypeOf(eid)
	if !exists {
		return false
	}

	// Remove the entity from the archetype.
	archetype := ws.archetypes[aid]
	archetype.removeEntity(eid)

	// Advance the generation so this ID no longer resolves. An index whose generation wraps is
	// retired rather than freed.
	record := &ws.entityArch[eid.index()]
	record.archetype = noArchetype
	record.generation++

	if record.generation != retiredGeneration {
		ws.pushFree(eid.index())
	}

	return true
}

// free is a min-heap, so newEntity always reuses the smallest index. A snapshot stores only the
// generations and restore rebuilds the free indices ascending, so reuse order must not depend on
// the order indices were freed. A heap rather than a sorted slice: inserting kept the list readable
// but cost an O(n) memmove per removal, which is milliseconds a tick once a shrunken world leaves a
// large free list behind.
func (ws *worldState) pushFree(index uint32) {
	ws.free = append(ws.free, index)
	for i := len(ws.free) - 1; i > 0; {
		parent := (i - 1) / 2
		if ws.free[parent] <= ws.free[i] {
			break
		}

		ws.free[parent], ws.free[i] = ws.free[i], ws.free[parent]
		i = parent
	}
}

// popFree removes and returns the smallest free index. The caller must check that free is non-empty.
func (ws *worldState) popFree() uint32 {
	smallest := ws.free[0]
	last := len(ws.free) - 1
	ws.free[0] = ws.free[last]
	ws.free = ws.free[:last]

	for i := 0; ; {
		left, right, lowest := 2*i+1, 2*i+2, i
		if left < len(ws.free) && ws.free[left] < ws.free[lowest] {
			lowest = left
		}

		if right < len(ws.free) && ws.free[right] < ws.free[lowest] {
			lowest = right
		}

		if lowest == i {
			return smallest
		}

		ws.free[i], ws.free[lowest] = ws.free[lowest], ws.free[i]
		i = lowest
	}
}

// moveEntity moves an entity to a new archetype with the given components. Returns a ponter to the
// destination archetype.
func (ws *worldState) moveEntity(eid EntityID, newComponents bitmap.Bitmap) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	oldAid, exists := ws.archetypeOf(eid)
	assert.That(exists, "entity doesn't exist. caller should've checked")

	newAid := ws.findOrCreateArchetype(newComponents)

	// Move the entity to the new oldArchetype.
	newArchetype := ws.archetypes[newAid]
	oldArchetype := ws.archetypes[oldAid]
	oldArchetype.moveEntity(newArchetype, eid)

	// Update the archetype mapping.
	ws.setArchetype(eid.index(), newAid)
}

// findOrCreateArchetype finds an existing archetype that matches the given components or creates a
// new one if no archetypes match.
// NOTE: findOrCreateArchetype has a chance of reallocating ws.archetypes, invalidating existing
// pointers to items in ws.archetypes. Be careful when using this method.
func (ws *worldState) findOrCreateArchetype(components bitmap.Bitmap) archetypeID {
	aid, exists := ws.archExact(components)
	if exists {
		return aid
	}

	// Create the new archetype with the desired components.
	aid = len(ws.archetypes)
	newArchetype := ws.newArchetype(aid, components)

	// Add it to the archetypes array.
	ws.archetypes = append(ws.archetypes, newArchetype)

	return aid
}

// -------------------------------------------------------------------------------------------------
// Component operations
// -------------------------------------------------------------------------------------------------

// setComponent sets a component in the given entity. Returns an error if the entity doesn't exist.
// If the entity's archetype contains the component type, this will update the value. If it doesn't,
// it will move the entity to a new archetype and set the value there.
func (ws *worldState) setComponent[T Component](eid EntityID, component T) error {
	aid, exists := ws.archetypeOf(eid)
	if !exists {
		return eris.Wrapf(ErrEntityNotFound, "entity %v", eid)
	}

	archetype := ws.archetypes[aid]

	// Resolve the component ID with a type-aware lookup so a name-only collision (an unregistered
	// Go type whose Name() matches a registered component of a different type) is rejected before
	// any archetype mutation. This matches the type-conflict handling in get/removeComponent.
	cid, err := ws.components.lookup(component.Name(), reflect.TypeFor[T]())
	if err != nil {
		return eris.Wrap(err, "failed to get component id")
	}

	// If current archetype doesnt' contain the component, move the entity to one that does.
	if !archetype.components.Contains(cid) {
		// Create the desired newComponents bitmap.
		newComponents := archetype.components.Clone(nil)
		newComponents.Set(cid)

		ws.moveEntity(eid, newComponents)

		// Update the archetype and row variable with the new archetype.
		newAid, newExists := ws.archetypeOf(eid)
		assert.That(newExists, "entity should exist after moveEntity")

		archetype = ws.archetypes[newAid]
	}

	// Get the column from the archetype directly.
	index := archetype.components.CountTo(cid)
	column, ok := archetype.columns[index].(*column[T])
	assert.That(ok, "unexpected column type")

	row, exists := archetype.rows.get(eid.index())
	assert.That(exists, "entity should have a row in its archetype")
	column.set(row, component)

	return nil
}

// getComponent gets a component value from the given entity. Returns an error if the entity doesn't
// exist or if the entity's archetype doesn't contain the component type.
func (ws *worldState) getComponent[T Component](eid EntityID) (T, error) {
	var zero T

	aid, exists := ws.archetypeOf(eid)
	if !exists {
		return zero, eris.Wrapf(ErrEntityNotFound, "entity %v", eid)
	}

	archetype := ws.archetypes[aid]

	cid, err := ws.components.getID(zero.Name())
	if err != nil {
		return zero, eris.Wrap(err, "failed to get component id")
	}

	if !archetype.components.Contains(cid) {
		return zero, eris.Errorf("entity %v doesn't contain component %s", eid, zero.Name())
	}

	// Get the column from the archetype directly.
	index := archetype.components.CountTo(cid)

	column, ok := archetype.columns[index].(*column[T])
	if !ok {
		return zero, eris.Wrapf(ErrComponentNotFound, "component %s is registered with a different type", zero.Name())
	}

	row, exists := archetype.rows.get(eid.index())
	assert.That(exists, "entity should have a row in its archetype")

	return column.get(row), nil
}

// removeComponent removes a component from the given entity. Returns an error if the entity or the
// component to remove doesn't exist.
func (ws *worldState) removeComponent[T Component](eid EntityID) error {
	var zero T

	aid, exists := ws.archetypeOf(eid)
	if !exists {
		return eris.Wrapf(ErrEntityNotFound, "entity %v", eid)
	}

	archetype := ws.archetypes[aid]

	cid, err := ws.components.getID(zero.Name())
	if err != nil {
		return eris.Wrap(err, "failed to get component id")
	}

	// Check if the entity actually has this component.
	if !archetype.components.Contains(cid) {
		// Entity doesn't have this component, nothing to remove
		return nil
	}

	// A matching name does not register a different Go type. Check before moving data.
	if _, ok := archetype.columns[archetype.components.CountTo(cid)].(*column[T]); !ok {
		return eris.Wrapf(ErrComponentNotFound, "component %s is registered with a different type", zero.Name())
	}

	// Create the components bitmap without the component to remove.
	newComponents := archetype.components.Clone(nil)
	newComponents.Remove(cid)

	// A remove component is basically a move, so just move the entity to the correct archetype.
	ws.moveEntity(eid, newComponents)

	return nil
}

// -------------------------------------------------------------------------------------------------
// Serialization
// -------------------------------------------------------------------------------------------------

// toProto captures an entity-major snapshot with independently encoded MessagePack payloads.
// The tick owns the world throughout capture. No live component data is retained by the result.
func (ws *worldState) toProto() (*cardinalv1.WorldState, error) {
	state := &cardinalv1.WorldState{
		NextId:      uint32(len(ws.entityArch)), //nolint:gosec // bounded by maxEntityIndex
		Components:  append([]string(nil), ws.components.names...),
		Generations: make([]uint32, len(ws.entityArch)),
	}
	for index, record := range ws.entityArch {
		state.Generations[index] = record.generation
		if record.archetype == noArchetype {
			continue
		}

		eid := newEntityID(uint32(index), record.generation)
		arch := ws.archetypes[record.archetype]

		row, ok := arch.rows.get(eid.index())
		if !ok {
			panic(fmt.Sprintf("snapshot: entity %v has an archetype but no row", eid))
		}

		entity := &cardinalv1.Entity{Id: uint64(eid)}

		arch.components.Range(func(cid uint32) {
			entity.Components = append(entity.Components, cid)
		})

		for _, col := range arch.columns {
			payload, err := col.encode(row)
			if err != nil {
				return nil, eris.Wrapf(err, "failed to snapshot entity %v", eid)
			}

			entity.Payloads = append(entity.Payloads, payload)
		}

		state.Entities = append(state.Entities, entity)
	}

	return state, nil
}

// maxRestoreFreeIDs is the maximum number of free IDs that a restore accepts. Without this limit,
// a large next_id can cause billions of free IDs.
const maxRestoreFreeIDs = 1 << 24 // 16.7M IDs, 12 bytes each (record and free entry): 192 MiB

// invalidComponentID identifies a name table entry that this build does not register.
const invalidComponentID ComponentID = maxComponentID + 1

// fromProto rebuilds the worldState from a decoded snapshot. It builds a new state first and
// commits it at the end. Thus a bad snapshot does not change the live world.
func (ws *worldState) fromProto(pb *cardinalv1.WorldState) error {
	if pb == nil {
		return eris.New("snapshot has no world state")
	}

	nextIndex := pb.GetNextId()

	// An unknown name is an error only if an entity uses it. Thus a removed component does not
	// prevent a restore.
	table := pb.GetComponents()
	tableCIDs := make([]ComponentID, len(table))

	var seen bitmap.Bitmap

	for i, name := range table {
		cid, err := ws.components.getID(name)
		if err != nil {
			tableCIDs[i] = invalidComponentID
			continue
		}

		if seen.Contains(cid) {
			return eris.Errorf("snapshot name table repeats component %q", name)
		}

		seen.Set(cid)
		tableCIDs[i] = cid
	}

	freeCount := int64(nextIndex) - int64(len(pb.GetEntities()))
	if freeCount < 0 {
		return eris.Errorf("snapshot has %d entities but next_id is only %d", len(pb.GetEntities()), nextIndex)
	}

	if freeCount > maxRestoreFreeIDs {
		return eris.Errorf("snapshot implies %d free entity ids, above the %d restore limit",
			freeCount, maxRestoreFreeIDs)
	}

	generations := pb.GetGenerations()
	if int64(len(generations)) != int64(nextIndex) {
		return eris.Errorf("snapshot has %d generations but next_id is %d", len(generations), nextIndex)
	}

	// A restore does not register components. Thus the new state uses the live component registry.
	next := &worldState{
		components: ws.components,
		entityArch: make([]entityRecord, nextIndex),
		archetypes: make([]*archetype, 1),
	}
	next.archetypes[voidArchetypeID] = next.newArchetype(voidArchetypeID, bitmap.Bitmap{})

	for index, generation := range generations {
		next.entityArch[index] = entityRecord{archetype: noArchetype, generation: generation}
	}

	prev := int64(-1)

	for _, ent := range pb.GetEntities() {
		eid := EntityID(ent.GetId())

		index := int64(eid.index())
		if index <= prev {
			return eris.Errorf("snapshot entities not strictly ascending at index %d", index)
		}

		if index >= int64(nextIndex) {
			return eris.Errorf("snapshot entity index %d is not below next_id %d", index, nextIndex)
		}

		prev = index

		if err := next.restoreEntity(eid, ent, table, tableCIDs); err != nil {
			return err
		}
	}

	next.free = freeIndices(next.entityArch, freeCount)

	// Commit the new state. The component registry and mutex do not change.
	ws.invalidateArchetypes()
	ws.entityArch, ws.free, ws.archetypes = next.entityArch, next.free, next.archetypes

	return nil
}

// freeIndices returns the unretired indices without an entity, ascending, which is a valid min-heap.
func freeIndices(entityArch []entityRecord, capacity int64) []uint32 {
	free := make([]uint32, 0, capacity)

	for index, record := range entityArch {
		if record.archetype == noArchetype && record.generation != retiredGeneration {
			free = append(free, uint32(index))
		}
	}

	return free
}

// restoreEntity creates one entity in the archetype for its component set. It decodes the
// payloads into that archetype. The caller has checked that the entity's index is in the table.
func (ws *worldState) restoreEntity(
	eid EntityID, ent *cardinalv1.Entity, table []string, tableCIDs []ComponentID,
) error {
	generation := ws.entityArch[eid.index()].generation
	if generation == retiredGeneration {
		return eris.Errorf("snapshot entity %v is at retired index %d", eid, eid.index())
	}

	if eid.generation() != generation {
		return eris.Errorf("snapshot entity %v has generation %d, but index %d has generation %d",
			eid, eid.generation(), eid.index(), generation)
	}

	idxs := ent.GetComponents()

	payloads := ent.GetPayloads()
	if len(idxs) != len(payloads) {
		return eris.Errorf("snapshot entity %v has %d component indices but %d payloads",
			eid, len(idxs), len(payloads))
	}

	var comps bitmap.Bitmap

	last := int64(-1)
	for _, idx := range idxs {
		if int64(idx) <= last {
			return eris.Errorf("snapshot entity %v component indices not strictly ascending", eid)
		}

		if int(idx) >= len(tableCIDs) {
			return eris.Errorf("snapshot entity %v component index %d outside the name table", eid, idx)
		}

		if tableCIDs[idx] == invalidComponentID {
			return eris.Errorf("snapshot entity %v holds component %q, which this build does not register",
				eid, table[idx])
		}

		last = int64(idx)
		comps.Set(tableCIDs[idx])
	}

	aid := ws.findOrCreateArchetype(comps)
	arch := ws.archetypes[aid]
	arch.newEntity(eid)
	ws.setArchetype(eid.index(), aid)

	row, ok := arch.rows.get(eid.index())
	assert.That(ok, "entity was just created in this archetype")

	for k, idx := range idxs {
		cid := tableCIDs[idx]

		col := arch.columns[arch.components.CountTo(cid)]
		if err := col.decode(row, payloads[k]); err != nil {
			return eris.Wrapf(err, "failed to restore entity %v", eid)
		}
	}

	return nil
}

// -------------------------------------------------------------------------------------------------
// Archetype helpers
// -------------------------------------------------------------------------------------------------

// newArchetype creates a new archetype with the given archetype ID and components bitmap.
func (ws *worldState) newArchetype(aid archetypeID, components bitmap.Bitmap) *archetype {
	count := components.Count()
	columns := make([]abstractColumn, count)

	// Initialize the columns with the column factories.
	index := 0

	components.Range(func(cid uint32) {
		factory := ws.components.factories[cid]
		columns[index] = factory()
		index++
	})
	assert.That(index == count, "not all columns are created")

	arch := newArchetype(aid, components, columns)

	return &arch
}

// archExact returns the archetype that exactly matches the given component types.
func (ws *worldState) archExact(components bitmap.Bitmap) (archetypeID, bool) {
	for aid, archetype := range ws.archetypes {
		if archetype.exact(components) {
			return aid, true
		}
	}

	return 0, false
}
