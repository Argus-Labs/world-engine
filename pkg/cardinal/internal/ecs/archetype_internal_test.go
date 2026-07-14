package ecs

import (
	"fmt"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing archetype operations
// -------------------------------------------------------------------------------------------------
// This test verifies the archetype implementation correctness by applying random sequences of
// operations and comparing it against a regular Go map of entity->archetype as the model.
// -------------------------------------------------------------------------------------------------

func TestArchetype_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax   = 1 << 15 // 32_768 iterations
		opNew    = "new"
		opMove   = "move"
		opRemove = "remove"
		// Max component count for the archetype pool. The number of archetypes generated is 2^n.
		nComponents = 10
	)

	// Randomize operation weights.
	operations := []string{opNew, opMove, opRemove}
	weights := testutils.RandOpWeights(prng, operations)

	pool := newArchetypePool(prng.IntN(nComponents))
	model := make(map[EntityID]*archetype) // Just track archetype entity ownership

	// List of live entities for random selection.
	var entities []EntityID
	var next EntityID

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opNew:
			eid := next
			next++
			entities = append(entities, eid)
			arch := pool[prng.IntN(len(pool))]

			arch.newEntity(eid)
			model[eid] = arch

			// Property: entity exists and bijection holds.
			row, exists := arch.rows.get(eid)
			assert.True(t, exists)
			assert.Equal(t, eid, arch.entities[row])

		case opRemove:
			if len(entities) == 0 {
				continue
			}
			idx := prng.IntN(len(entities))
			eid := entities[idx]
			arch := model[eid]

			arch.removeEntity(eid)
			delete(model, eid)
			entities = slices.Delete(entities, idx, idx+1)

			// Property: entity no longer exists.
			_, exists := arch.rows.get(eid)
			assert.False(t, exists)

		case opMove:
			if len(entities) == 0 {
				continue
			}
			eid := entities[prng.IntN(len(entities))]
			src := model[eid]
			dst := pool[prng.IntN(len(pool))]

			src.moveEntity(dst, eid)

			if src == dst {
				// Property: self move is a no-op and the model remains unchanged.
				row, exists := src.rows.get(eid)
				assert.True(t, exists)
				assert.Equal(t, eid, src.entities[row])
				assert.Same(t, src, model[eid])
			} else {
				model[eid] = dst

				// Property: entity no longer exists in source.
				_, exists := src.rows.get(eid)
				assert.False(t, exists)

				// Property: entity exists in destination and bijection holds.
				row, exists := dst.rows.get(eid)
				assert.True(t, exists)
				assert.Equal(t, eid, dst.entities[row])
			}

		default:
			panic("unreachable")
		}
	}

	// Property: bijection holds for all archetypes.
	// Bijection means there's a 1-1 mapping of entity->row. Every entity maps to a unique row, and
	// every row comes from a unique entity.
	for _, arch := range pool {
		// Forward: entities[i] -> rows
		// Property: rows.get(entities[i]) == i
		for i, eid := range arch.entities {
			row, exists := arch.rows.get(eid)
			// Property: rows.get(entities[i]) must exist.
			assert.True(t, exists, "entity %d at index %d not found in rows", eid, i)
			assert.Equal(t, i, row, "entity %d at index %d has row %d", eid, i, row)
		}
		// Reverse: rows -> entities[row]. This catches stale entries in rows that forward check would miss.
		// Property: entities[row] == eid.
		for eid, row := range arch.rows {
			if row == sparseTombstone {
				continue
			}
			assert.Less(t, row, len(arch.entities), "row %d for entity %d out of bounds", row, eid)
			assert.Equal(t, EntityID(eid), arch.entities[row], "entities[%d] != %d", row, eid)
		}
	}

	// Property: entity IDs are globally unique (also catches duplicates within a single archetype).
	seenGlobal := make(map[EntityID]bool)
	for _, arch := range pool {
		for _, eid := range arch.entities {
			assert.False(t, seenGlobal[eid], "duplicate entity %d", eid)
			seenGlobal[eid] = true
		}
	}

	// Property: all entities in archetypes match the entities list.
	assert.Len(t, seenGlobal, len(entities), "entity count mismatch")
	for _, eid := range entities {
		assert.True(t, seenGlobal[eid], "entity %d in list but not in any archetype", eid)
	}

	// Final state check: archetype entity ownership matches model.
	for eid, expectedArch := range model {
		row, exists := expectedArch.rows.get(eid)
		assert.True(t, exists, "entity %d should exist in model's archetype", eid)
		if exists {
			assert.Equal(t, eid, expectedArch.entities[row], "entity %d mismatch in model's archetype", eid)
		}
	}
}

// -------------------------------------------------------------------------------------------------
// Exhaustive archetype move test
// -------------------------------------------------------------------------------------------------
// This test exhaustively enumerates all combinations of source (S) and destination (D) archetypes
// and entity positions to verify moveEntity correctness. Move diagnostics classify each transition
// by component delta (k):
//
//   | Category                   | Condition     | Example            |
//   | -------------------------- | ------------- | ------------------ |
//   | nop                        | A=0, R=0      | {} -> {}, {A}->{A} |
//   | add only (without copy)    | A>0, R=0, I=0 | {} -> {A,B}        |
//   | add only (with copy)       | A>0, R=0, I>0 | {A} -> {A,B}       |
//   | remove only (without copy) | A=0, R>0, I=0 | {A} -> {}          |
//   | remove only (with copy)    | A=0, R>0, I>0 | {A,B,C} -> {A,C}   |
//   | add+remove (without copy)  | A>0, R>0, I=0 | {A,B} -> {C,D}     |
//   | add+remove (with copy)     | A>0, R>0, I>0 | {A,B} -> {B,C}     |
//
// where A is the number of components added in D, R is the number removed from S, and I is the
// number copied from S to D (intersection).
//
// Self moves (S == D) are included and only assert that moveEntity does not panic (no-op path).
//
// For each S->D pair, we also vary entity count (1-3) and move position (first/middle/last) to
// exercise swap-remove edge cases: first and middle trigger a swap, last only truncates.
// We also verify that shared component data is copied correctly to the destination.
// -------------------------------------------------------------------------------------------------

func TestArchetype_MoveExhaustive(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const maxComponents = 6

	// Generate all possible subsets (archetype) with N component types, total values: 2^N.
	pool := newArchetypePool(maxComponents)

	// Exhaustively test combinations of source * destination * entity count * moved entity row.
	gen := testutils.NewGen()
	for !gen.Done() {
		// Pick and reset archetypes to clean state.
		src := pool[gen.Index(len(pool))] // Randomize source
		src.reset()
		dst := pool[gen.Index(len(pool))] // Randomize destination
		dst.reset()

		// Populate source with 1-3 entities, destination with 0-2 entities.
		countSrc := gen.Range(1, 3) // Randomize source entity count
		for eid := range countSrc {
			src.newEntity(EntityID(eid))
		}
		countDst := gen.Range(0, 2) // Randomize destination entity count
		for eid := range countDst {
			dst.newEntity(EntityID(countSrc + eid)) // Entity IDs are globally unique
		}

		// Pick which entity to move.
		row := gen.Index(len(src.entities)) // Randomize moved entity row
		eid := src.entities[row]

		// Uncomment to see all cases checked.
		// t.Logf("move: %s", classifyMove(src, dst, row))

		if src == dst {
			assert.NotPanics(t, func() {
				src.moveEntity(dst, eid)
			}, "self move should be a no-op and must not panic")
			continue
		}

		// Set non-zero values on the entity's components before the move.
		for _, col := range src.columns {
			col.setAbstract(row, testutils.SimpleComponent{Value: prng.Int()})
		}
		// This is used to check if the components are correctly copied over.
		sourceValuesByName := make(map[string]Component, len(src.columns))
		for _, col := range src.columns {
			sourceValuesByName[col.name()] = col.getAbstract(row)
		}

		// Record original state.
		srcLenBefore := len(src.entities)
		dstLenBefore := len(dst.entities)

		src.moveEntity(dst, eid)

		// Property: entity no longer exists in source.
		_, exists := src.rows.get(eid)
		assert.False(t, exists, "entity %d should not exist in source after move", eid)

		// Property: entity exists in destination and bijection holds.
		dstRow, exists := dst.rows.get(eid)
		assert.True(t, exists, "entity %d should exist in destination after move", eid)
		assert.Equal(t, eid, dst.entities[dstRow], "bijection broken: dst.entities[%d] != %d", dstRow, eid)

		// Property: source entity count decreased by 1.
		assert.Len(t, src.entities, srcLenBefore-1, "source entity count should decrease by 1")

		// Property: destination entity count increased by 1.
		assert.Len(t, dst.entities, dstLenBefore+1, "destination entity count should increase by 1")

		// Property: bijection holds for all remaining entities in source.
		for i, e := range src.entities {
			r, ok := src.rows.get(e)
			assert.True(t, ok, "entity %d should exist in source rows", e)
			assert.Equal(t, i, r, "bijection broken in source: rows[%d] = %d, expected %d", e, r, i)
		}

		// Property: bijection holds for all entities in destination.
		for i, e := range dst.entities {
			r, ok := dst.rows.get(e)
			assert.True(t, ok, "entity %d should exist in destination rows", e)
			assert.Equal(t, i, r, "bijection broken in destination: rows[%d] = %d, expected %d", e, r, i)
		}

		// Property: shared components are copied correctly.
		for _, dstCol := range dst.columns {
			isShared := slices.ContainsFunc(src.columns, func(c abstractColumn) bool {
				return c.name() == dstCol.name()
			})
			if isShared {
				expected := sourceValuesByName[dstCol.name()]
				actual := dstCol.getAbstract(dstRow)
				assert.Equal(t, expected, actual, "shared component %s not copied", dstCol.name())
			}
		}
	}
}

func newArchetypePool(n int) []*archetype {
	pool := make([]bitmap.Bitmap, 0, 1<<n)
	for mask := range 1 << n {
		components := bitmap.Bitmap{}
		for bit := range uint32(n) {
			if mask&(1<<bit) != 0 {
				components.Set(bit)
			}
		}
		pool = append(pool, components)
	}

	archetypes := make([]*archetype, 0, len(pool))
	for aid, components := range pool {
		columns := make([]abstractColumn, components.Count())
		for i := range components.Count() {
			columns[i] = newColumnFactory[testutils.SimpleComponent]()()
		}
		arch := newArchetype(aid, components, columns)
		archetypes = append(archetypes, &arch)
	}

	return archetypes
}

// classifyMove returns semantic class and diagnostics for exhaustive move tests.
//
// Variables:
//   - S, D: source/destination archetype IDs
//   - A: number of components added when moving S -> D
//   - R: number of components removed when moving S -> D
//   - I: number of components copied (intersection)
//   - k: component difference between S and D (A + R)
func classifyMove(src, dst *archetype, movedRow int) string { //nolint:unused // useful for debugging test cases
	intersection := src.components.Clone(nil)
	intersection.And(dst.components)

	copied := intersection.Count() // I
	added := dst.compCount - copied
	removed := src.compCount - copied
	diff := added + removed // k

	var category string
	switch {
	case added == 0 && removed == 0:
		category = "nop"
	case added > 0 && removed == 0 && copied == 0:
		category = "add only (without copy)"
	case added > 0 && removed == 0 && copied > 0:
		category = "add only (with copy)"
	case added == 0 && removed > 0 && copied == 0:
		category = "remove only (without copy)"
	case added == 0 && removed > 0 && copied > 0:
		category = "remove only (with copy)"
	case copied == 0:
		category = "add+remove (without copy)"
	default:
		category = "add+remove (with copy)"
	}

	return fmt.Sprintf(
		"%s | S=%d D=%d A=%d R=%d I=%d k=%d S_entities=%d D_entities=%d moved_row=%d",
		category,
		src.id,
		dst.id,
		added,
		removed,
		copied,
		diff,
		len(src.entities),
		len(dst.entities),
		movedRow,
	)
}

// -------------------------------------------------------------------------------------------------
// Serialization smoke test
// -------------------------------------------------------------------------------------------------
// We don't extensively test toProto/fromProto because:
// 1. The implementation delegates to column and sparse set serialization (tested separately).
// 2. The remaining logic is straightforward type conversion loops.
// 3. Heavy property-based testing would mostly exercise the underlying serialization, not our code.
// -------------------------------------------------------------------------------------------------

func TestArchetype_SerializationSmoke(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const entityMax = 1000

	arch, cm := newSimpleArchetype(t)
	entityCount := prng.IntN(entityMax) + 1
	for eid := range entityCount {
		arch.newEntity(EntityID(eid))
		for _, col := range arch.columns {
			col.setAbstract(eid, testutils.SimpleComponent{Value: eid + 1})
		}
	}

	pb, err := arch.toProto()
	require.NoError(t, err)

	arch2 := &archetype{}
	err = arch2.fromProto(pb, cm)
	require.NoError(t, err)

	// Property: deserialize(serialize(x)) == x.
	assertArchetypeEqual(t, arch, arch2)
}

func newSimpleArchetype(t *testing.T) (*archetype, *componentManager) {
	t.Helper()

	cm := newComponentManager()
	cid, err := cm.register(
		testutils.SimpleComponent{}.Name(),
		newColumnFactory[testutils.SimpleComponent](),
	)
	require.NoError(t, err)

	components := bitmap.Bitmap{}
	components.Set(cid)
	archValue := newArchetype(0, components, []abstractColumn{
		newColumnFactory[testutils.SimpleComponent]()(),
	})

	return &archValue, &cm
}

// -------------------------------------------------------------------------------------------------
// Deserialization edge case regression test
// -------------------------------------------------------------------------------------------------
// This guards against the case where fromProto panics if a bitmap's length isn't a multiple of 8.
// To reproduce, run TestWorld_DeserializeNegative with:
// - Seed: 0x187f45843d5f9288
// - Commit: f79096323ed10ae364e6cab6a7fffd430885443c
// -------------------------------------------------------------------------------------------------

func TestArchetype_DeserializationNegative(t *testing.T) {
	t.Parallel()

	arch, cm := newSimpleArchetype(t)
	arch.newEntity(0)

	pb, err := arch.toProto()
	require.NoError(t, err)

	// Corrupt the bitmap by truncating to an invalid length (not a multiple of 8).
	pb.ComponentsBitmap = pb.GetComponentsBitmap()[:len(pb.GetComponentsBitmap())-1]

	arch2 := &archetype{}
	err = arch2.fromProto(pb, cm)

	// Property: corrupted bitmap should return an error, not panic.
	assert.Error(t, err)
}

// assertArchetypeEqual checks if two archetypes are struturally equal. This function is extracted
// so it can be reused in serialization tests "above" this layer.
func assertArchetypeEqual(t *testing.T, a1, a2 *archetype) {
	t.Helper()

	assert.Equal(t, a1.id, a2.id)
	assert.Equal(t, a1.components.ToBytes(), a2.components.ToBytes())
	assert.Equal(t, a1.entities, a2.entities)
	assert.Equal(t, a1.rows, a2.rows)

	assert.Len(t, a2.columns, len(a1.columns))
	for i := range a1.columns {
		c1, c2 := a1.columns[i], a2.columns[i]
		assert.Equal(t, c1.len(), c2.len())
		for row := range c1.len() {
			assert.Equal(t, c1.getAbstract(row), c2.getAbstract(row))
		}
	}
}
