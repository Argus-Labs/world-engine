package ecs

import (
	"math"
	"slices"
	"sort"
	"strings"
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

// reset clears all entity data while preserving registered components.
func (ws *worldState) reset() {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.nextID = 0
	ws.free = ws.free[:0]
	ws.entityArch.clear()
	ws.archetypes = ws.archetypes[:1] // Keep the void archetype slot
	// Reset the void archetype to avoid stale data.
	ws.archetypes[voidArchetypeID] = ws.newArchetype(voidArchetypeID, bitmap.Bitmap{})
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

	row, exists := archetype.rows.get(eid)
	assert.That(exists, "entity should have a row in its archetype")
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

// -------------------------------------------------------------------------------------------------
// Serialization
// -------------------------------------------------------------------------------------------------

// toProto converts the worldState to its wire form: entities and their components by name,
// nothing of the runtime layout. Sorting (columns by name, ID lists ascending) is part of the
// format protobuf's deterministic marshal doesn't sort repeated fields, so identical worlds
// producing identical bytes happens here or not at all.
func (ws *worldState) toProto() (*cardinalv1.WorldState, error) {
	live := make([]uint32, 0)
	merged := make(map[string]*cardinalv1.Column)

	for _, arch := range ws.archetypes {
		for _, eid := range arch.entities { // Track all entities to not lose any void entities.
			live = append(live, uint32(eid))
		}
		// Merge each archetype's columns into one column per component name.
		for _, col := range arch.columns {
			payloads, err := col.encodeRows()
			if err != nil {
				return nil, eris.Wrapf(err, "failed to serialize column %q", col.name())
			}
			pbCol := merged[col.name()]
			if pbCol == nil {
				pbCol = &cardinalv1.Column{Name: col.name()}
				merged[col.name()] = pbCol
			}
			for row, payload := range payloads {
				pbCol.EntityIds = append(pbCol.EntityIds, uint32(arch.entities[row]))
				pbCol.Payloads = append(pbCol.Payloads, payload)
			}
		}
	}
	slices.Sort(live)

	columns := make([]*cardinalv1.Column, 0, len(merged))
	for _, pbCol := range merged {
		sort.Sort(columnSorter{pbCol})
		columns = append(columns, pbCol)
	}
	slices.SortFunc(columns, func(a, b *cardinalv1.Column) int {
		return strings.Compare(a.GetName(), b.GetName())
	})

	return &cardinalv1.WorldState{
		NextId:        uint32(ws.nextID),
		LiveEntityIds: live,
		Columns:       columns,
	}, nil
}

// columnSorter sorts a column's (entity, payload) pairs by entity ID, swapping both slices in
// lockstep so the pairing cannot drift.
type columnSorter struct{ c *cardinalv1.Column }

func (s columnSorter) Len() int           { return len(s.c.GetEntityIds()) }
func (s columnSorter) Less(i, j int) bool { return s.c.GetEntityIds()[i] < s.c.GetEntityIds()[j] }
func (s columnSorter) Swap(i, j int) {
	s.c.EntityIds[i], s.c.EntityIds[j] = s.c.GetEntityIds()[j], s.c.GetEntityIds()[i]
	s.c.Payloads[i], s.c.Payloads[j] = s.c.GetPayloads()[j], s.c.GetPayloads()[i]
}

// pendingComponent is one component awaiting restore: which type, and its encoded value.
type pendingComponent struct {
	cid  ComponentID
	data []byte
}

// fromProto rebuilds the worldState from its wire form. The file only says which entities have
// which components; archetypes, rows and the entity-archetype index are whatever this rebuild
// produces. More work than rehydrating structs, but it's a boot-time path.
func (ws *worldState) fromProto(pb *cardinalv1.WorldState) error {
	nextID := EntityID(pb.GetNextId())

	liveSet, err := validateLiveList(pb.GetLiveEntityIds(), nextID)
	if err != nil {
		return err
	}
	// Collect all components from the columns mapped by entity ID.
	pending, err := ws.collectColumns(pb.GetColumns(), liveSet)
	if err != nil {
		return err
	}

	// Reset to just the void archetype, then create every live entity directly in the archetype its
	// component set implies. Ascending order keeps the rebuilt runtime deterministic for a given file.
	ws.nextID = nextID
	ws.archetypes = make([]*archetype, 1)
	ws.archetypes[voidArchetypeID] = ws.newArchetype(voidArchetypeID, bitmap.Bitmap{})
	ws.entityArch = newSparseSet()

	for _, eid := range pb.GetLiveEntityIds() {
		if err := ws.restoreEntity(EntityID(eid), pending[eid]); err != nil {
			return err
		}
	}

	// The free list is derived: every ID below next_id that is not alive, ascending.
	ws.free = ws.free[:0]
	for eid := uint32(0); EntityID(eid) < nextID; eid++ {
		if _, ok := liveSet[eid]; !ok {
			ws.free = append(ws.free, EntityID(eid))
		}
	}
	return nil
}

// validateLiveList checks the live list before anything touches the world: strictly ascending
// (dupes and disorder are producer bugs, not tolerable variants) and below next_id.
func validateLiveList(liveIDs []uint32, nextID EntityID) (map[uint32]struct{}, error) {
	liveSet := make(map[uint32]struct{}, len(liveIDs))
	for i, eid := range liveIDs {
		if EntityID(eid) >= nextID {
			return nil, eris.Errorf("live entity %d is not below next_id %d", eid, nextID)
		}
		if i > 0 && liveIDs[i-1] >= eid {
			return nil, eris.Errorf("live_entity_ids not strictly ascending at index %d", i)
		}
		liveSet[eid] = struct{}{}
	}
	return liveSet, nil
}

// collectColumns gathers each entity's components from the wire columns. Unknown names and columns
// referencing entities outside the live list are hard errors: silently skipping either would lose
// saved data with no record.
func (ws *worldState) collectColumns(
	columns []*cardinalv1.Column, liveSet map[uint32]struct{},
) (map[uint32][]pendingComponent, error) {
	pending := make(map[uint32][]pendingComponent, len(liveSet))
	for _, pbCol := range columns {
		name := pbCol.GetName()
		ids := pbCol.GetEntityIds()
		payloads := pbCol.GetPayloads()

		cid, err := ws.components.getID(name)
		if err != nil {
			return nil, eris.Wrapf(err, "snapshot column %q does not match any registered component", name)
		}
		if len(ids) != len(payloads) { // A column must pair every entity with exactly one payload.
			return nil, eris.Errorf("column %q has %d entities but %d payloads", name, len(ids), len(payloads))
		}

		for i, eid := range ids {
			if _, ok := liveSet[eid]; !ok {
				return nil, eris.Errorf("column %q references entity %d which is not in live_entity_ids", name, eid)
			}
			pending[eid] = append(pending[eid], pendingComponent{cid: cid, data: payloads[i]})
		}
	}
	return pending, nil
}

// restoreEntity creates one entity directly in the archetype its component set implies and decodes
// its component values into place.
func (ws *worldState) restoreEntity(eid EntityID, components []pendingComponent) error {
	var comps bitmap.Bitmap
	for _, pc := range components {
		comps.Set(pc.cid)
	}

	aid := ws.findOrCreateArchetype(comps)
	arch := ws.archetypes[aid]
	arch.newEntity(eid)
	ws.entityArch.set(eid, aid)

	row, ok := arch.rows.get(eid)
	assert.That(ok, "entity was just created in this archetype")
	for _, pc := range components {
		if err := arch.columnByID(pc.cid).decodeRow(row, pc.data); err != nil {
			return eris.Wrapf(err, "failed to restore entity %d", eid)
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
