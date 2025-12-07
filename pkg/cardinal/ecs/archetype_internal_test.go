package ecs

import (
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
// This test verifies the archetype implementation correctness using model-based testing. It
// compares our implementation against Go's map tracking entity->archetype ownership by applying
// random sequences of new/move/remove operations to both and asserting equivalence.
// We also verify extra invariants such as bijection consistency and global entity uniqueness.
// -------------------------------------------------------------------------------------------------

func TestArchetype_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	pool := newArchetypePool()
	model := make(map[EntityID]*archetype) // Just track archetype entity ownership

	// List of live entities for random selection.
	var entities []EntityID
	var next EntityID

	for range opsMax {
		op := testutils.RandWeightedOp(prng, archetypeOps)
		switch op {
		case a_new:
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

		case a_remove:
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

		case a_move:
			if len(entities) == 0 {
				continue
			}
			eid := entities[prng.IntN(len(entities))]
			src := model[eid]
			dst := pool[prng.IntN(len(pool))]
			for dst == src { // Make sure dst != src
				dst = pool[prng.IntN(len(pool))]
			}

			src.moveEntity(dst, eid)
			model[eid] = dst

			// Property: entity no longer exists in source.
			_, exists := src.rows.get(eid)
			assert.False(t, exists)

			// Property: entity exists in destination and bijection holds.
			row, exists := dst.rows.get(eid)
			assert.True(t, exists)
			assert.Equal(t, eid, dst.entities[row])

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

type archetypeOp uint8

const (
	a_new    archetypeOp = 40
	a_move   archetypeOp = 35
	a_remove archetypeOp = 25
)

var archetypeOps = []archetypeOp{a_new, a_move, a_remove}

// -------------------------------------------------------------------------------------------------
// Exhaustive archetype move test
// -------------------------------------------------------------------------------------------------
// This test exhaustively enumerates all combinations of source/destination archetypes and entity
// positions to verify moveEntity correctness. The archetype pool covers all component relationship
// scenarios between source and destination:
//
//   | Scenario            | Examples                                  |
//   | ------------------- |------------------------------------------ |
//   | Empty -> non-empty  | {} -> {A}, {} -> {A,B}                    |
//   | Non-empty -> empty  | {A} -> {}, {A,B} -> {}                    |
//   | Subset -> superset  | {A} -> {A,B}                              |
//   | Superset -> subset  | {A,B} -> {A}                              |
//   | Partial overlap     | {A,B} -> {B,C}, {B,C} -> {A,B} (B shared) |
//   | Disjoint            | {A} -> {B,C}, {B,C} -> {A} (no shared)    |
//
// For each src->dst pair, we also vary entity count (1-3) and move position (first/middle/last)
// to exercise swap-remove edge cases: first and middle trigger a swap, last only truncates.
// We also verify that shared component data is copied correctly to the destination.
// -------------------------------------------------------------------------------------------------

func TestArchetype_MoveExhaustive(t *testing.T) {
	t.Parallel()

	// Exhaustively test combinations of source * destination * entity count * moved entity row.
	gen := testutils.NewGen()
	for !gen.Done() {
		pool := newArchetypePool()
		srcIdx := gen.Index(len(pool)) // Randomize source
		dstIdx := gen.Index(len(pool)) // Randomize destination
		if srcIdx == dstIdx {
			// Skip moves to self, this is a nop
			continue
		}
		src := pool[srcIdx]
		dst := pool[dstIdx]

		// Populate source with 1-3 entities.
		entityCount := gen.Range(1, 3) // Randomize entity count
		for eid := range entityCount {
			src.newEntity(EntityID(eid))
		}

		// Pick which entity to move.
		row := gen.Index(len(src.entities)) // Randomize moved entity row
		eid := src.entities[row]

		// Uncomment to see all cases checked.
		// t.Logf("move: %s, index %d of %d", classifyMove(srcIdx, dstIdx), row, entityCount)

		// Set non-zero values on the entity's components before the move.
		for _, col := range src.columns {
			col.setAbstract(row, testValueFor(col.name()))
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
				expected := testValueFor(dstCol.name())
				actual := dstCol.getAbstract(dstRow)
				assert.Equal(t, expected, actual, "shared component %s not copied", dstCol.name())
			}
		}
	}
}

const (
	cidA uint32 = 0
	cidB uint32 = 1
	cidC uint32 = 2
)

func newArchetypePool() []*archetype {
	return []*archetype{
		newTestArchetype(0, []uint32{}),           // {}
		newTestArchetype(1, []uint32{cidA}),       // {A}
		newTestArchetype(2, []uint32{cidA, cidB}), // {A, B}
		newTestArchetype(3, []uint32{cidB, cidC}), // {B, C}
	}
}

func newTestArchetype(aid archetypeID, cids []uint32) *archetype {
	components := bitmap.Bitmap{}
	columns := make([]abstractColumn, len(cids))

	for i, cid := range cids {
		components.Set(cid)
		switch cid {
		case cidA:
			columns[i] = newColumnFactory[testutils.ComponentA]()()
		case cidB:
			columns[i] = newColumnFactory[testutils.ComponentB]()()
		case cidC:
			columns[i] = newColumnFactory[testutils.ComponentC]()()
		}
	}

	arch := newArchetype(aid, components, columns)
	return &arch
}

func testValueFor(name string) Component {
	// We use fixed values rather than random because we're testing that moveEntity copies data
	// correctly—a structural operation with no value-dependent logic. If copying works for one
	// non-zero value, it works for all values.
	switch name {
	case "component_a":
		return testutils.ComponentA{X: 1.1, Y: 2.2, Z: 3.3}
	case "component_b":
		return testutils.ComponentB{ID: 42, Label: "test", Enabled: true}
	case "component_c":
		return testutils.ComponentC{Values: [8]int32{1, 2, 3, 4, 5, 6, 7, 8}, Counter: 99}
	default:
		panic("unknown component: " + name)
	}
}

// classifyMove returns a string describing the move scenario based on archetype indices.
// This is a helper function to visualize the cases handled in the test above.
func classifyMove(srcIdx, dstIdx int) string { //nolint:unused // useful for debugging test cases
	names := []string{"{}", "{A}", "{A,B}", "{B,C}"}
	label := names[srcIdx] + " -> " + names[dstIdx]

	// Pool: 0={}, 1={A}, 2={A,B}, 3={B,C}
	switch {
	case srcIdx == 0:
		return label + " (empty -> non-empty)"
	case dstIdx == 0:
		return label + " (non-empty -> empty)"
	case srcIdx == 1 && dstIdx == 2:
		return label + " (subset -> superset)"
	case srcIdx == 2 && dstIdx == 1:
		return label + " (superset -> subset)"
	case (srcIdx == 2 && dstIdx == 3) || (srcIdx == 3 && dstIdx == 2):
		return label + " (partial overlap)"
	case (srcIdx == 1 && dstIdx == 3) || (srcIdx == 3 && dstIdx == 1):
		return label + " (disjoint)"
	default:
		return label + " (unknown)"
	}
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

	cm := newComponentManager()
	cid1, err := cm.register(testutils.ComponentA{}.Name(), newColumnFactory[testutils.ComponentA]())
	require.NoError(t, err)
	cid2, err := cm.register(testutils.ComponentB{}.Name(), newColumnFactory[testutils.ComponentB]())
	require.NoError(t, err)

	arch := newTestArchetype(0, []uint32{cid1, cid2})
	entityCount := prng.IntN(entityMax) + 1
	for eid := range entityCount {
		arch.newEntity(EntityID(eid))
		for _, col := range arch.columns {
			col.setAbstract(eid, testValueFor(col.name()))
		}
	}

	pb, err := arch.toProto()
	require.NoError(t, err)

	arch2 := &archetype{}
	err = arch2.fromProto(pb, &cm)
	require.NoError(t, err)

	// Property: deserialize(serialize(x)) == x.
	assertArchetypeEqual(t, arch, arch2)
}

// assertArchetypeEqual checks if two archetypes are struturally equal. This function is extracted
// so it can be reused in serialization tests "above" this layer.
func assertArchetypeEqual(t *testing.T, a1, a2 *archetype) {
	t.Helper()

	assert.Equal(t, a1.id, a2.id)
	assert.Equal(t, a1.components.ToBytes(), a2.components.ToBytes())
	assert.Equal(t, a1.entities, a2.entities)
	assert.Equal(t, sparseSet(a1.rows), sparseSet(a2.rows))

	assert.Len(t, a2.columns, len(a1.columns))
	for i := range a1.columns {
		c1, c2 := a1.columns[i], a2.columns[i]
		assert.Equal(t, c1.len(), c2.len())
		for row := range c1.len() {
			assert.Equal(t, c1.getAbstract(row), c2.getAbstract(row))
		}
	}
}
