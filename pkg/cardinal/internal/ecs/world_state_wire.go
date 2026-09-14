package ecs

import (
	"fmt"

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

// stateWire is the encoder's cross-pass state, owned by worldState and touched only by the tick
// goroutine.
type stateWire struct {
	// pendingSize is the body size wireBodySize computed, for appendWireBody to enforce, and -1
	// when no size pass is outstanding.
	pendingSize int
}

// wireBodySize computes the exact encoded size of the WorldState message, and remembers it for
// appendWireBody to verify against.
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

// entityWireSize is the encoded size of one Entity message body.
func (ws *worldState) entityWireSize(arch *archetype, eid EntityID) int {
	row, ok := arch.rows.get(eid)
	if !ok {
		// Not an assert: under the release tag a miss would encode row 0 under this id, and both
		// passes would agree, so the length check cannot catch it.
		panic(fmt.Sprintf("snapshot: entity %d has an archetype but no row", eid))
	}

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

// appendWireBody writes the WorldState message the preceding wireBodySize call measured. The world
// must not change between the two.
func (ws *worldState) appendWireBody(buf []byte) []byte {
	assert.That(ws.wire.pendingSize >= 0, "appendWireBody called without a preceding wireBodySize call")
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

	// Length only: a same-length change passes. Not an assert — those vanish under the release tag.
	if len(buf)-start != ws.wire.pendingSize {
		panic("snapshot body length diverged from the size pass: the world changed size between the two passes")
	}
	ws.wire.pendingSize = -1
	return buf
}

// appendEntityWire writes one Entity message, tag and length included.
func (ws *worldState) appendEntityWire(buf []byte, arch *archetype, eid EntityID) []byte {
	row, ok := arch.rows.get(eid)
	if !ok {
		// Not an assert: under the release tag a miss would encode row 0 under this id, and both
		// passes would agree, so the length check cannot catch it.
		panic(fmt.Sprintf("snapshot: entity %d has an archetype but no row", eid))
	}

	// Recomputed rather than remembered: SizeWire is arithmetic for a generated component. Called
	// twice more below, so three times per component per snapshot.
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
		return buf
	}

	buf = protowire.AppendTag(buf, 2, protowire.BytesType)
	buf = protowire.AppendVarint(buf, uint64(packed)) //nolint:gosec // sizes are non-negative
	for _, cid := range arch.wireCIDs {
		buf = protowire.AppendVarint(buf, uint64(cid))
	}

	for _, col := range arch.columns {
		buf = protowire.AppendTag(buf, 3, protowire.BytesType)
		buf = protowire.AppendVarint(buf, uint64(col.rowWireSize(row))) //nolint:gosec // sizes are non-negative
		buf = col.appendRowWire(buf, row)
	}
	return buf
}

// -------------------------------------------------------------------------------------------------
// Restore
// -------------------------------------------------------------------------------------------------

// maxRestoreFreeIDs caps the free ids a snapshot may imply. They are the gaps below next_id, so a
// few bytes can ask for billions. Each costs ~20B: entityArch is sized by the highest id, not by
// the live count.
const maxRestoreFreeIDs = 1 << 24 // 16.7M ids, ~320 MiB

// invalidComponentID marks a name-table slot naming a component this build does not register.
const invalidComponentID ComponentID = maxComponentID + 1

// fromProto rebuilds the worldState from the decoded snapshot. Archetypes and index tables are
// whatever this rebuild produces. It builds into a scratch state and commits only once the whole
// file is accepted, so a bad snapshot leaves the live world untouched.
func (ws *worldState) fromProto(pb *cardinalv1.WorldState) error {
	if pb == nil {
		return eris.New("snapshot has no world state")
	}

	nextID := EntityID(pb.GetNextId())

	// Slots resolve by name, so table order carries no meaning. The writer emits every registered
	// component, so an unknown name only matters if an entity indexes it — mark it and let
	// restoreEntity report it, or dropping an unused component would break its own snapshots.
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

	// Every id below next_id that has no entity is free, so the count is known before the walk. It is
	// also the one number in the file that can ask for unbounded work, hence the bound.
	freeCount := int64(nextID) - int64(len(pb.GetEntities()))
	if freeCount < 0 {
		return eris.Errorf("snapshot has %d entities but next_id is only %d", len(pb.GetEntities()), nextID)
	}
	if freeCount > maxRestoreFreeIDs {
		return eris.Errorf("snapshot implies %d free entity ids, above the %d restore limit",
			freeCount, maxRestoreFreeIDs)
	}

	// Scratch state: the component registry is shared (restore never registers), everything else is
	// built fresh and swapped in at the end.
	next := &worldState{
		components: ws.components,
		nextID:     nextID,
		entityArch: newSparseSet(),
		archetypes: make([]*archetype, 1),
	}
	next.archetypes[voidArchetypeID] = next.newArchetype(voidArchetypeID, bitmap.Bitmap{})
	next.free = make([]EntityID, 0, freeCount) // exact, so the gap fill never regrows

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

	// Commit. wire and mu stay as they are; only the rebuilt state moves across.
	ws.nextID, ws.free, ws.entityArch, ws.archetypes = next.nextID, next.free, next.entityArch, next.archetypes
	return nil
}

// restoreEntity creates one entity directly in the archetype its component set implies and decodes
// its component values into place.
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
