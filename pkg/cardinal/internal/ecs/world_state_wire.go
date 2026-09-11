package ecs

import (
	"github.com/argus-labs/world-engine/pkg/assert"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/encoding/protowire"
)

// Wire encoding writes the WorldState message directly from ECS memory into one buffer —
// no intermediate proto graph, no maps, no sorting, no translation. Two passes over the same
// ascending entityArch scan: wireBodySize computes the exact encoded size (protobuf writes a
// length before anything variable-sized, so sizes must be known first), appendWireBody writes
// the bytes front-to-back and asserts it wrote exactly what the size pass computed.
//
// The file's order IS the runtime's order, so the hot loop just reads:
//   - entities ascend because entityArch is an array indexed by entity ID — the scan index is the order
//   - the name table is every registered component in registration order — which is component-ID
//     order, so a component's table index is its ID and no lookup exists
//   - each entity's components ascend because archetype columns are stored in component-ID order
//
// The file stays self-describing: restore resolves table entries by NAME (see fromProto), never by
// this build's numbering, so registration-order files load correctly across code changes. What
// registration order must be is deterministic per build — it is: registration happens in explicit
// call order, never map iteration — or identical worlds would stop producing identical bytes.
//
// Fallback components (no generated SizeWire/AppendWire yet) still allocate inside MarshalWire —
// see column.rowWireSize.

// stateWire is the encoder's cross-pass state, owned by worldState and touched only by the tick
// goroutine.
type stateWire struct {
	// pendingSize is the body size staged by wireBodySize for appendWireBody to enforce,
	// -1 when no size pass is staged. The two passes must observe identical world state; the
	// asserts downstream of this are what turn a mutation between them into a crash instead of a
	// corrupt snapshot.
	pendingSize int
}

// wireBodySize computes the exact encoded size of the WorldState message and stages the
// snapshot: every fallback component is pre-encoded, and the result is remembered for
// appendWireBody to verify against. Encoding cannot fail: a component that cannot marshal asserts
// inside column.rowWireSize rather than reporting an error nobody could act on.
func (ws *worldState) wireBodySize() int {
	n := 0
	if ws.nextID != 0 {
		n += protowire.SizeTag(1) + protowire.SizeVarint(uint64(ws.nextID))
	}

	// Name table: every registered component in registration order, so a component's table index
	// is its ID. Entities reference these by index instead of repeating the name.
	for _, name := range ws.components.names {
		n += protowire.SizeTag(2) + protowire.SizeBytes(len(name))
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

	ws.wire.pendingSize = n
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
	if len(arch.columns) == 0 {
		return n // void entity: id only
	}

	// Field 2: component table indices, packed. A component's table index is its ID.
	packed := 0
	for _, cid := range arch.wireCIDs {
		packed += protowire.SizeVarint(uint64(cid))
	}
	n += protowire.SizeTag(2) + protowire.SizeBytes(packed)

	// Field 3: one payload per component, in the same order as the indices.
	for _, col := range arch.columns {
		n += protowire.SizeTag(3) + protowire.SizeBytes(col.rowWireSize(row))
	}
	return n
}

// appendWireBody writes the WorldState message staged by the wireBodySize call directly
// before it. The world must not change between the two calls; the final assert is what catches it
// if it does.
func (ws *worldState) appendWireBody(buf []byte) []byte {
	assert.That(ws.wire.pendingSize >= 0, "appendWireBody called without a staging wireBodySize call")
	start := len(buf)

	if ws.nextID != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(ws.nextID))
	}

	for _, name := range ws.components.names {
		buf = protowire.AppendTag(buf, 2, protowire.BytesType)
		buf = protowire.AppendString(buf, name)
	}

	for eid := EntityID(0); eid < ws.nextID; eid++ {
		aid, ok := ws.entityArch.get(eid)
		if !ok {
			continue
		}
		buf = ws.appendEntityWire(buf, ws.archetypes[aid], eid)
	}

	assert.That(len(buf)-start == ws.wire.pendingSize,
		"snapshot bytes diverged from the size pass: the world changed between the two passes")
	ws.wire.pendingSize = -1
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
	if len(arch.columns) > 0 { // if entity has components
		for _, cid := range arch.wireCIDs {
			packed += protowire.SizeVarint(uint64(cid))
		}
		inner += protowire.SizeTag(2) + protowire.SizeBytes(packed)
		for _, col := range arch.columns {
			inner += protowire.SizeTag(3) + protowire.SizeBytes(col.stagedRowWireSize(row))
		}
	}

	buf = protowire.AppendTag(buf, 3, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(inner)) //nolint:gosec // sizes are non-negative

	if eid != 0 {
		buf = protowire.AppendTag(buf, 1, protowire.VarintType)
		buf = protowire.AppendVarint(buf, uint64(eid))
	}
	if len(arch.columns) == 0 {
		return buf
	}

	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(packed)) //nolint:gosec // sizes are non-negative
	for _, cid := range arch.wireCIDs {
		buf = protowire.AppendVarint(buf, uint64(cid))
	}

	for _, col := range arch.columns {
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

	// Resolve the name table. Every name must be unique and match a registered component —
	// restoring past an unknown name would lose saved data with no record. Table order is the
	// writer's registration order and carries no meaning here: slots resolve by name, which is
	// what keeps old files loading after components are added or removed.
	table := pb.GetComponents()
	tableCIDs := make([]ComponentID, len(table))
	var seen bitmap.Bitmap
	for i, name := range table {
		cid, err := ws.components.getID(name)
		if err != nil {
			return eris.Wrapf(err, "snapshot component %q does not match any registered component", name)
		}
		if seen.Contains(cid) {
			return eris.Errorf("snapshot name table repeats component %q", name)
		}
		seen.Set(cid)
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
