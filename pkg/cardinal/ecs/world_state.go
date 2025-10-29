package ecs

import (
	"math"
	"sync"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
)

// EntityID is a unique identifier for an entity.
type EntityID uint32

// maxEntityID is the maximum entity ID that can be created.
const maxEntityID = math.MaxUint32 - 1

// invalidEntityID is a sentinel id for errors or when we have exceeded the maximum entities count.
const invalidEntityID = maxEntityID + 1

// voidArchetype is an archetype without components.
const voidArchetypeID = 0

// worldState holds the state of the world.
type worldState struct {
	components componentManager // Component type manager
	nextID     EntityID         // Entity ID counter
	free       []EntityID       // Free entity IDs to reuse
	entityArch sparseSet
	archetypes []*archetype // Array of archetypes
	mu         sync.Mutex
}

// newWorldState creates a new world state.
func newWorldState() *worldState {
	ws := worldState{
		components: newComponentManager(),
		nextID:     0,
		free:       make([]EntityID, 0),
		entityArch: newSparseSet(),
		archetypes: make([]*archetype, 1),
	}

	// Insert the void archetype.
	ws.archetypes[voidArchetypeID] = ws.newArchetype(voidArchetypeID, bitmap.Bitmap{})

	return &ws
}

// -------------------------------------------------------------------------------------------------
// Entity operations
// -------------------------------------------------------------------------------------------------

// newEntity creates a new entity of the void archetype in the world state. Returns the entity ID.
func (ws *worldState) newEntity() EntityID {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	var eid EntityID
	if len(ws.free) > 0 { // Reuse free IDs if any
		eid = ws.free[0]
		ws.free = ws.free[1:]
	} else { // Else get the next ID
		eid = ws.nextID
		ws.nextID++
	}
	assert.That(eid != invalidEntityID, "max number of entities exceeded")

	// New entities are assigned to the void archetype, which doesn't contain any components.
	voidArchetype := ws.archetypes[voidArchetypeID]
	// Add the entity to the void archetype.
	voidArchetype.newEntity(eid)

	// Update the entity archetype mapping.
	ws.entityArch.set(eid, voidArchetypeID)

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

	aid, exists := ws.entityArch.get(eid)
	if !exists {
		return false
	}

	// Remove the entity from the archetype.
	archetype := ws.archetypes[aid]
	archetype.removeEntity(eid)

	// Remove the removed entity ID from the map.
	ok := ws.entityArch.remove(eid)
	assert.That(ok, "entity isn't removed from sparse set")

	// Add the removed ID to the free list for reuse.
	ws.free = append(ws.free, eid)

	return true
}

// moveEntity moves an entity to a new archetype with the given components. Returns a ponter to the
// destination archetype.
func (ws *worldState) moveEntity(eid EntityID, newComponents bitmap.Bitmap) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	oldAid, exists := ws.entityArch.get(eid)
	assert.That(exists, "entity doesn't exist. caller should've checked")

	newAid := ws.findOrCreateArchetype(newComponents)

	// Move the entity to the new oldArchetype.
	newArchetype := ws.archetypes[newAid]
	oldArchetype := ws.archetypes[oldAid]
	oldArchetype.moveEntity(newArchetype, eid)

	// Update the archetype mapping.
	ws.entityArch.set(eid, newAid)
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
func setComponent[T Component](ws *worldState, eid EntityID, component T) error {
	aid, exists := ws.entityArch.get(eid)
	if !exists {
		return eris.Wrapf(ErrEntityNotFound, "entity %d", eid)
	}
	archetype := ws.archetypes[aid]

	cid, err := ws.components.getID(component.Name())
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
		newAid, newExists := ws.entityArch.get(eid)
		assert.That(newExists, "entity should exist after moveEntity")
		archetype = ws.archetypes[newAid]
	}

	// Get the column from the archetype directly.
	index := archetype.components.CountTo(cid)
	column, ok := archetype.columns[index].(*column[T])
	assert.That(ok, "unexpected column type")

	row, exists := archetype.rows.get(eid)
	assert.That(exists, "entity should have a row in its archetype")
	column.set(row, component)
	return nil
}

// getComponent gets a component value from the given entity. Returns an error if the entity doesn't
// exist or if the entity's archetype doesn't contain the component type.
func getComponent[T Component](ws *worldState, eid EntityID) (T, error) {
	var zero T

	aid, exists := ws.entityArch.get(eid)
	if !exists {
		return zero, eris.Wrapf(ErrEntityNotFound, "entity %d", eid)
	}
	archetype := ws.archetypes[aid]

	cid, err := ws.components.getID(zero.Name())
	if err != nil {
		return zero, eris.Wrap(err, "failed to get component id")
	}

	if !archetype.components.Contains(cid) {
		return zero, eris.Errorf("entity %d doesn't contain component %s", eid, zero.Name())
	}

	// Get the column from the archetype directly.
	index := archetype.components.CountTo(cid)
	column, ok := archetype.columns[index].(*column[T])
	assert.That(ok, "unexpected column type")

	row := archetype.rows[eid]
	return column.get(row), nil
}

// removeComponent removes a component from the given entity. Returns an error if the entity or the
// component to remove doesn't exist.
func removeComponent[T Component](ws *worldState, eid EntityID) error {
	var zero T

	aid, exists := ws.entityArch.get(eid)
	if !exists {
		return eris.Wrapf(ErrEntityNotFound, "entity %d", eid)
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

	// Create the components bitmap without the component to remove.
	newComponents := archetype.components.Clone(nil)
	newComponents.Remove(cid)

	// A remove component is basically a move, so just move the entity to the correct archetype.
	ws.moveEntity(eid, newComponents)
	return nil
}

// registerComponent registers a component type with the world state.
func registerComponent[T Component](ws *worldState) (componentID, error) {
	var zero T
	return ws.components.register(zero.Name(), newColumnFactory[T]())
}

// -------------------------------------------------------------------------------------------------
// Serialization
// -------------------------------------------------------------------------------------------------

// serialize converts the worldState to a protobuf message for serialization.
func (ws *worldState) serialize() (*cardinalv1.CardinalSnapshot, error) {
	freeIDs := make([]uint32, len(ws.free))
	for i, entityID := range ws.free {
		freeIDs[i] = uint32(entityID)
	}

	pbArchetypes := make([]*cardinalv1.Archetype, len(ws.archetypes))
	for i, arch := range ws.archetypes {
		pbArch, err := arch.serialize()
		if err != nil {
			return nil, eris.Wrapf(err, "failed to serialize archetype %d", i)
		}
		pbArchetypes[i] = pbArch
	}

	return &cardinalv1.CardinalSnapshot{
		NextId:     uint32(ws.nextID),
		FreeIds:    freeIDs,
		EntityArch: ws.entityArch.serialize(),
		Archetypes: pbArchetypes,
	}, nil
}

// deserialize populates the worldState from a protobuf message.
func (ws *worldState) deserialize(pb *cardinalv1.CardinalSnapshot) error {
	ws.nextID = EntityID(pb.GetNextId())

	ws.free = make([]EntityID, len(pb.GetFreeIds()))
	for i, freeID := range pb.GetFreeIds() {
		ws.free[i] = EntityID(freeID)
	}

	ws.entityArch.deserialize(pb.GetEntityArch())

	ws.archetypes = make([]*archetype, len(pb.GetArchetypes()))
	for i, pbArch := range pb.GetArchetypes() {
		ws.archetypes[i] = &archetype{}
		if err := ws.archetypes[i].deserialize(pbArch, &ws.components); err != nil {
			return eris.Wrapf(err, "failed to deserialize archetype %d", i)
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

// archContains returns all archetypes that have the given component types.
func (ws *worldState) archContains(components bitmap.Bitmap) []int {
	result := make([]int, 0)
	for aid, archetype := range ws.archetypes {
		if archetype.contains(components) {
			result = append(result, aid)
		}
	}
	return result
}
