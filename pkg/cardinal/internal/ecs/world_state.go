package ecs

import (
	"math"
	"sync"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/encoding/protowire"
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
	wire       stateWire    // Encoder state between the two passes. Only the tick goroutine uses it.
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
		wire:       stateWire{pendingSize: -1}, // No size pass is pending.
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
	if len(ws.free) > 0 { // Reuse the smallest free id (see pushFree)
		eid = ws.popFree()
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

	ws.pushFree(eid)

	return true
}

// free is a min-heap, so newEntity always reuses the smallest id. A snapshot stores only the gaps
// and restore rebuilds them ascending, so reuse order must not depend on the order ids were freed.
// A heap rather than a sorted slice: inserting kept the list readable but cost an O(n) memmove per
// removal, which is milliseconds a tick once a shrunken world leaves a large free list behind.
func (ws *worldState) pushFree(eid EntityID) {
	ws.free = append(ws.free, eid)
	for i := len(ws.free) - 1; i > 0; {
		parent := (i - 1) / 2
		if ws.free[parent] <= ws.free[i] {
			break
		}
		ws.free[parent], ws.free[i] = ws.free[i], ws.free[parent]
		i = parent
	}
}

// popFree removes and returns the smallest free id. The caller must check that free is non-empty.
func (ws *worldState) popFree() EntityID {
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
func (ws *worldState) setComponent[T Component](eid EntityID, component T) error {
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
func (ws *worldState) getComponent[T Component](eid EntityID) (T, error) {
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
func (ws *worldState) removeComponent[T Component](eid EntityID) error {
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

// The encoder writes a snapshot in two passes. wireBodySize measures the message. appendWireBody
// writes the message. The size pass comes first because protobuf puts the length of a message
// before its content. The encoder does not sort. Entity IDs, the component name table, and
// archetype columns are already in file order. A restore matches components by name, not by ID.

// stateWire is the encoder state between the size pass and the write pass.
type stateWire struct {
	pendingSize int // The size from wireBodySize. -1 means that no size pass is pending.
}

// wireBodySize returns the encoded size of the WorldState message. It stores the size for
// appendWireBody to check.
func (ws *worldState) wireBodySize() (int, error) {
	n := 0
	if ws.nextID != 0 {
		n += protowire.SizeTag(1) + protowire.SizeVarint(uint64(ws.nextID))
	}

	// The name table is in registration order. Thus the table index of a component is its ID.
	for _, name := range ws.components.names {
		n += protowire.SizeTag(2) + protowire.SizeBytes(len(name))
	}

	for eid := EntityID(0); eid < ws.nextID; eid++ {
		aid, ok := ws.entityArch.get(eid)
		if !ok {
			continue // The ID is free.
		}
		size, err := ws.entityWireSize(ws.archetypes[aid], eid)
		if err != nil {
			return 0, err
		}
		n += protowire.SizeTag(3) + protowire.SizeBytes(size)
	}

	ws.wire.pendingSize = n
	return n, nil
}

// entityWireSize returns the encoded size of one Entity message body.
func (ws *worldState) entityWireSize(arch *archetype, eid EntityID) (int, error) {
	row, ok := arch.rows.get(eid)
	if !ok {
		return 0, eris.Errorf("snapshot: entity %d has an archetype but no row", eid)
	}

	n := 0
	if eid != 0 {
		n += protowire.SizeTag(1) + protowire.SizeVarint(uint64(eid))
	}
	if len(arch.columns) == 0 {
		return n, nil
	}

	// Field 2 is a packed list of table indices. Each index is a component ID.
	packed := 0
	arch.components.Range(func(cid uint32) {
		packed += protowire.SizeVarint(uint64(cid))
	})
	n += protowire.SizeTag(2) + protowire.SizeBytes(packed)

	// Field 3 is one payload for each component, in the same order.
	for _, col := range arch.columns {
		n += protowire.SizeTag(3) + protowire.SizeBytes(col.rowWireSize(row))
	}
	return n, nil
}

// appendWireBody writes the WorldState message. It returns an error if an entity has no row or
// the written length differs from the size pass. The world must not change between the two passes.
func (ws *worldState) appendWireBody(buf []byte) ([]byte, error) {
	assert.That(ws.wire.pendingSize >= 0, "appendWireBody called without a preceding wireBodySize call")
	want := ws.wire.pendingSize
	ws.wire.pendingSize = -1
	start := len(buf)

	if ws.nextID != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(ws.nextID))
	}

	for _, name := range ws.components.names {
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendString(buf, name)
	}

	var err error
	for eid := EntityID(0); eid < ws.nextID; eid++ {
		aid, ok := ws.entityArch.get(eid)
		if !ok {
			continue
		}
		buf, err = ws.appendEntityWire(buf, ws.archetypes[aid], eid)
		if err != nil {
			return nil, err
		}
	}

	if got := len(buf) - start; got != want {
		return nil, eris.Errorf("snapshot: body wrote %d bytes, size pass computed %d", got, want)
	}
	return buf, nil
}

// appendEntityWire writes one Entity message with its tag and length.
func (ws *worldState) appendEntityWire(buf []byte, arch *archetype, eid EntityID) ([]byte, error) {
	row, ok := arch.rows.get(eid)
	if !ok {
		return nil, eris.Errorf("snapshot: entity %d has an archetype but no row", eid)
	}

	// This function computes the sizes again. A cache is not necessary because rowWireSize is fast.
	inner := 0
	if eid != 0 {
		inner += protowire.SizeTag(1) + protowire.SizeVarint(uint64(eid))
	}
	packed := 0
	if len(arch.columns) > 0 {
		arch.components.Range(func(cid uint32) {
			packed += protowire.SizeVarint(uint64(cid))
		})
		inner += protowire.SizeTag(2) + protowire.SizeBytes(packed)
		for _, col := range arch.columns {
			inner += protowire.SizeTag(3) + protowire.SizeBytes(col.rowWireSize(row))
		}
	}

	buf = protowire.AppendTag(buf, 3, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(inner)) //nolint:gosec // sizes are non-negative

	if eid != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(eid))
	}
	if len(arch.columns) == 0 {
		return buf, nil
	}

	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(packed)) //nolint:gosec // sizes are non-negative
	arch.components.Range(func(cid uint32) {
		buf = protowire.AppendVarint(buf, uint64(cid))
	})

	for _, col := range arch.columns {
		buf = protowire.AppendTag(buf, 3, protowire.BytesType)
		buf = protowire.AppendVarint(buf, uint64(col.rowWireSize(row))) //nolint:gosec // sizes are non-negative
		buf = col.appendRowWire(buf, row)
	}
	return buf, nil
}

// maxRestoreFreeIDs is the maximum number of free IDs that a restore accepts. Without this limit,
// a large next_id can cause billions of free IDs.
const maxRestoreFreeIDs = 1 << 24 // 16.7M IDs, about 320 MiB

// invalidComponentID identifies a name table entry that this build does not register.
const invalidComponentID ComponentID = maxComponentID + 1

// fromProto rebuilds the worldState from a decoded snapshot. It builds a new state first and
// commits it at the end. Thus a bad snapshot does not change the live world.
func (ws *worldState) fromProto(pb *cardinalv1.WorldState) error {
	if pb == nil {
		return eris.New("snapshot has no world state")
	}

	nextID := EntityID(pb.GetNextId())

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

	freeCount := int64(nextID) - int64(len(pb.GetEntities()))
	if freeCount < 0 {
		return eris.Errorf("snapshot has %d entities but next_id is only %d", len(pb.GetEntities()), nextID)
	}
	if freeCount > maxRestoreFreeIDs {
		return eris.Errorf("snapshot implies %d free entity ids, above the %d restore limit",
			freeCount, maxRestoreFreeIDs)
	}

	// A restore does not register components. Thus the new state uses the live component registry.
	next := &worldState{
		components: ws.components,
		nextID:     nextID,
		entityArch: newSparseSet(),
		archetypes: make([]*archetype, 1),
	}
	next.archetypes[voidArchetypeID] = next.newArchetype(voidArchetypeID, bitmap.Bitmap{})
	next.free = make([]EntityID, 0, freeCount)

	// The entities are in ascending order. The gaps between them are the free IDs.
	prev := int64(-1)
	for _, ent := range pb.GetEntities() {
		eid := int64(ent.GetId())
		if eid <= prev {
			return eris.Errorf("snapshot entities not strictly ascending at id %d", eid)
		}
		if eid >= int64(nextID) {
			return eris.Errorf("snapshot entity %d is not below next_id %d", eid, nextID)
		}
		for gap := prev + 1; gap < eid; gap++ {
			next.free = append(next.free, EntityID(gap)) //nolint:gosec // bounded below nextID
		}
		prev = eid

		if err := next.restoreEntity(EntityID(eid), ent, table, tableCIDs); err != nil { //nolint:gosec // bounded
			return err
		}
	}
	for gap := prev + 1; gap < int64(nextID); gap++ {
		next.free = append(next.free, EntityID(gap)) //nolint:gosec // bounded below nextID
	}

	// Commit the new state. wire and mu do not change.
	ws.nextID, ws.free, ws.entityArch, ws.archetypes = next.nextID, next.free, next.entityArch, next.archetypes
	return nil
}

// restoreEntity creates one entity in the archetype for its component set. It decodes the
// payloads into that archetype.
func (ws *worldState) restoreEntity(
	eid EntityID, ent *cardinalv1.Entity, table []string, tableCIDs []ComponentID,
) error {
	idxs := ent.GetComponents()
	payloads := ent.GetPayloads()
	if len(idxs) != len(payloads) {
		return eris.Errorf("snapshot entity %d has %d component indices but %d payloads",
			eid, len(idxs), len(payloads))
	}

	var comps bitmap.Bitmap
	last := int64(-1)
	for _, idx := range idxs {
		if int64(idx) <= last {
			return eris.Errorf("snapshot entity %d component indices not strictly ascending", eid)
		}
		if int(idx) >= len(tableCIDs) {
			return eris.Errorf("snapshot entity %d component index %d outside the name table", eid, idx)
		}
		if tableCIDs[idx] == invalidComponentID {
			return eris.Errorf("snapshot entity %d holds component %q, which this build does not register",
				eid, table[idx])
		}
		last = int64(idx)
		comps.Set(tableCIDs[idx])
	}

	aid := ws.findOrCreateArchetype(comps)
	arch := ws.archetypes[aid]
	arch.newEntity(eid)
	ws.entityArch.set(eid, aid)

	row, ok := arch.rows.get(eid)
	assert.That(ok, "entity was just created in this archetype")
	for k, idx := range idxs {
		cid := tableCIDs[idx]
		col := arch.columns[arch.components.CountTo(cid)]
		if err := col.decodeRow(row, payloads[k]); err != nil {
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
