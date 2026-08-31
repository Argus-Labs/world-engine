package ecs

import (
	"slices"
	"strings"

	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/encoding/protowire"
)

// Wire encoding writes the WorldState message directly from ECS memory into one buffer —
// no intermediate proto graph, no maps, no sorting at snapshot time. Two passes over the same
// ascending entityArch scan: wireBodySize computes the exact encoded size (protobuf writes a
// length before anything variable-sized, so sizes must be known first), appendWireBody writes
// the bytes front-to-back and asserts it wrote exactly what the size pass computed.
//
// Every ordering rule the format demands is precomputed on a cold path, so the hot loop just reads:
//   - entities ascend because entityArch is an array indexed by entity ID — the scan index is the order
//   - each entity's components are name-sorted via archetype.wireOrder, computed at archetype creation
//   - the name table is every registered component, name-sorted via sortedCIDs, computed once
//     after registration
//
// The scratch below is reused across snapshots and grows to a high-water mark, the same memory
// policy as the rest of the runtime. Fallback components (no generated SizeWire/AppendWire yet)
// still allocate inside MarshalWire — see column.rowWireSize.

// stateWire is the reusable encoder scratch, owned by worldState and touched only by the tick
// goroutine.
type stateWire struct {
	sortedCIDs []ComponentID // every registered component ID, name-ascending
	tableIdx   []uint32      // cid -> index in the name table (its position in sortedCIDs)

	// pendingSize is the body size staged by wireBodySize for appendWireBody to enforce,
	// -1 when no size pass is staged. The two passes must observe identical world state; the
	// asserts downstream of this are what turn a mutation between them into a crash instead of a
	// corrupt snapshot.
	pendingSize int
}

// prepare builds sortedCIDs and tableIdx on first use, then becomes a no-op: the length check
// passes on every later call, so the snapshot hot path pays one comparison.
func (w *stateWire) prepare(cm *componentManager) {
	if len(w.sortedCIDs) == len(cm.names) {
		return
	}
	w.sortedCIDs = slices.Grow(w.sortedCIDs[:0], len(cm.names))
	for cid := range cm.names {
		w.sortedCIDs = append(w.sortedCIDs, ComponentID(cid)) //nolint:gosec // bounded by registry size
	}
	slices.SortFunc(w.sortedCIDs, func(a, b ComponentID) int {
		return strings.Compare(cm.names[a], cm.names[b])
	})
	w.tableIdx = slices.Grow(w.tableIdx[:0], len(cm.names))[:len(cm.names)]
	for slot, cid := range w.sortedCIDs {
		w.tableIdx[cid] = uint32(slot) //nolint:gosec // bounded by registry size
	}
}

// wireBodySize computes the exact encoded size of the WorldState message and stages the
// snapshot: every fallback component is pre-encoded, and the result is remembered for
// appendWireBody to verify against. Encoding cannot fail: a component that cannot marshal asserts
// inside column.rowWireSize rather than reporting an error nobody could act on.
func (ws *worldState) wireBodySize() int {
	w := &ws.wire
	w.prepare(&ws.components)

	n := 0
	if ws.nextID != 0 {
		n += protowire.SizeTag(1) + protowire.SizeVarint(uint64(ws.nextID))
	}

	// Name table: every registered component in name order. Entities reference these by index
	// (tableIdx, filled in prepare) instead of repeating the name.
	for _, cid := range w.sortedCIDs {
		n += protowire.SizeTag(2) + protowire.SizeBytes(len(ws.components.names[cid]))
	}

	// Entities: one ascending scan of the entity->archetype index. The scan index is the entity
	// ID, so the file's "strictly ascending" rule costs a for loop.
	for eid := EntityID(0); eid < ws.nextID; eid++ {
		aid, ok := ws.entityArch.get(eid)
		if !ok {
			continue // dead ID
		}
		size := ws.entityWireSize(ws.archetypes[aid], eid)
		n += protowire.SizeTag(3) + protowire.SizeBytes(size)
	}

	w.pendingSize = n
	return n
}

// entityWireSize is the encoded size of one Entity message body. It also stages fallback
// components (see column.rowWireSize).
func (ws *worldState) entityWireSize(arch *archetype, eid EntityID) int {
	row, ok := arch.rows.get(eid)
	assert.That(ok, "entity has an archetype but no row")

	n := 0
	if eid != 0 {
		n += protowire.SizeTag(1) + protowire.SizeVarint(uint64(eid))
	}
	if len(arch.wireOrder) == 0 {
		return n // void entity: id only
	}

	// Field 2: component table indices, packed.
	packed := 0
	for _, cid := range arch.wireCIDs {
		packed += protowire.SizeVarint(uint64(ws.wire.tableIdx[cid]))
	}
	n += protowire.SizeTag(2) + protowire.SizeBytes(packed)

	// Field 3: one payload per component, in the same name order.
	for _, colIdx := range arch.wireOrder {
		n += protowire.SizeTag(3) + protowire.SizeBytes(arch.columns[colIdx].rowWireSize(row))
	}
	return n
}

// appendWireBody writes the WorldState message staged by the wireBodySize call directly
// before it. The world must not change between the two calls; the final assert is what catches it
// if it does.
func (ws *worldState) appendWireBody(buf []byte) []byte {
	w := &ws.wire
	assert.That(w.pendingSize >= 0, "appendWireBody called without a staging wireBodySize call")
	start := len(buf)

	if ws.nextID != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(ws.nextID))
	}

	for _, cid := range w.sortedCIDs {
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendString(buf, ws.components.names[cid])
	}

	for eid := EntityID(0); eid < ws.nextID; eid++ {
		aid, ok := ws.entityArch.get(eid)
		if !ok {
			continue
		}
		buf = ws.appendEntityWire(buf, ws.archetypes[aid], eid)
	}

	assert.That(len(buf)-start == w.pendingSize,
		"snapshot bytes diverged from the size pass: the world changed between the two passes")
	w.pendingSize = -1
	return buf
}

// appendEntityWire writes one Entity message, tag and length included.
func (ws *worldState) appendEntityWire(buf []byte, arch *archetype, eid EntityID) []byte {
	row, ok := arch.rows.get(eid)
	assert.That(ok, "entity has an archetype but no row")

	// The entity's body size, recomputed the same way the size pass did. Direct components rerun
	// SizeWire (pure arithmetic); fallback components read their staged bytes.
	inner := 0
	if eid != 0 {
		inner += protowire.SizeTag(1) + protowire.SizeVarint(uint64(eid))
	}
	packed := 0
	if len(arch.wireOrder) > 0 { // if entity has components
		for _, cid := range arch.wireCIDs {
			packed += protowire.SizeVarint(uint64(ws.wire.tableIdx[cid]))
		}
		inner += protowire.SizeTag(2) + protowire.SizeBytes(packed)
		for _, colIdx := range arch.wireOrder {
			inner += protowire.SizeTag(3) + protowire.SizeBytes(arch.columns[colIdx].stagedRowWireSize(row))
		}
	}

	buf = protowire.AppendTag(buf, 3, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(inner)) //nolint:gosec // sizes are non-negative

	if eid != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(eid))
	}
	if len(arch.wireOrder) == 0 {
		return buf
	}

	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(packed)) //nolint:gosec // sizes are non-negative
	for _, cid := range arch.wireCIDs {
		buf = protowire.AppendVarint(buf, uint64(ws.wire.tableIdx[cid]))
	}

	for _, colIdx := range arch.wireOrder {
		col := arch.columns[colIdx]
		buf = protowire.AppendTag(buf, 3, protowire.BytesType)
		buf = protowire.AppendVarint(buf, uint64(col.stagedRowWireSize(row))) //nolint:gosec // sizes are non-negative
		buf = col.appendRowWire(buf, row)
	}
	return buf
}

// -------------------------------------------------------------------------------------------------
// Restore
// -------------------------------------------------------------------------------------------------

// fromProto rebuilds the worldState from the decoded snapshot message. The file only says which
// entities have which components; archetypes and the index tables are whatever this rebuild
// produces. A boot path: allocations here are fine, invalid input is a hard error before or during
// the rebuild, never a silent skip.
func (ws *worldState) fromProto(pb *cardinalv1.WorldState) error {
	nextID := EntityID(pb.GetNextId())

	// Resolve the name table. Strictly ascending is a format rule, and every name must match a
	// registered component — restoring past an unknown name would lose saved data with no record.
	table := pb.GetComponents()
	tableCIDs := make([]ComponentID, len(table))
	for i, name := range table {
		if i > 0 && table[i-1] >= name {
			return eris.Errorf("snapshot name table not strictly ascending at index %d", i)
		}
		cid, err := ws.components.getID(name)
		if err != nil {
			return eris.Wrapf(err, "snapshot component %q does not match any registered component", name)
		}
		tableCIDs[i] = cid
	}

	// Reset to just the void archetype, keeping the component registry.
	ws.nextID = nextID
	ws.archetypes = make([]*archetype, 1)
	ws.archetypes[voidArchetypeID] = ws.newArchetype(voidArchetypeID, bitmap.Bitmap{})
	ws.entityArch = newSparseSet()
	ws.free = ws.free[:0]

	// Entities arrive strictly ascending, so the free list is the gaps — filled in the same pass.
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
			ws.free = append(ws.free, EntityID(gap)) //nolint:gosec // bounded below nextID
		}
		prev = eid

		if err := ws.restoreEntity(EntityID(eid), ent, tableCIDs); err != nil { //nolint:gosec // bounded
			return err
		}
	}
	for gap := prev + 1; gap < int64(nextID); gap++ {
		ws.free = append(ws.free, EntityID(gap)) //nolint:gosec // bounded below nextID
	}
	return nil
}

// restoreEntity creates one entity directly in the archetype its component set implies and decodes
// its component values into place.
func (ws *worldState) restoreEntity(eid EntityID, ent *cardinalv1.Entity, tableCIDs []ComponentID) error {
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
