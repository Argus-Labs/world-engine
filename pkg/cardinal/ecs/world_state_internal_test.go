package ecs

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing world state operations
//
// This test verifies the worldState implementation correctness using model-based testing. It
// compares our implementation against a map[EntityID]map[string]Component as the model by applying
// random sequences of entity and component operations to both and asserting equivalence.
// We also verify structural invariants: entity-archetype bijection and global entity uniqueness.
// -------------------------------------------------------------------------------------------------

func TestWorldState_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const opsMax = 1 << 15 // 32_768 iterations

	impl := newTestWorldState(t)
	model := make(map[EntityID]map[string]any)

	for range opsMax {
		op := testutils.RandWeightedOp(prng, worldStateOps)
		switch op {
		case ws_entityNew:
			eid := impl.newEntity()
			model[eid] = make(map[string]any)

			// Property: new entity should exist in entityArch.
			_, exists := impl.entityArch.get(eid)
			assert.True(t, exists, "newEntity(%d) should exist in entityArch", eid)

		case ws_entityRemove:
			eid := EntityID(prng.IntN(10_000)) // Default to random (which might not exist).
			// Bias toward existing entities (80%) to test actual removal path.
			if len(model) > 0 && prng.Float64() < 0.8 {
				eid = testutils.RandMapKey(prng, model)
			}

			implOk := impl.removeEntity(eid)
			_, modelOk := model[eid]
			delete(model, eid)

			// Property: removeEntity returns same existence as model.
			assert.Equal(t, modelOk, implOk, "removeEntity(%d) existence mismatch", eid)

			// Property: entity no longer exists in entityArch after removal.
			_, exists := impl.entityArch.get(eid)
			assert.False(t, exists, "removeEntity(%d) should not exist in entityArch", eid)

		case ws_compSetUpdate:
			if len(model) == 0 {
				continue
			}
			// Find an entity that has at least one component.
			eid := testutils.RandMapKey(prng, model)
			existingComponents := model[eid]
			if len(existingComponents) == 0 {
				continue
			}

			// Pick a component the entity already has and update it.
			name := testutils.RandMapKey(prng, existingComponents)
			c := randComponentByName(prng, name)
			aidBefore, ok := impl.entityArch.get(eid)
			assert.True(t, ok, "<TODO FILL ME IN>")

			setComponentAbstract(t, impl, eid, c)
			model[eid][c.Name()] = c

			// Property: archetype should NOT change (update in place).
			aidAfter, exists := impl.entityArch.get(eid)
			assert.True(t, exists, "setComponentUpdate(%d) entity should exist", eid)
			assert.Equal(t, aidBefore, aidAfter, "setComponentUpdate(%d) archetype should not change", eid)

		case ws_compSetMove:
			if len(model) == 0 {
				continue
			}
			// Find an entity and a component it doesn't have.
			eid := testutils.RandMapKey(prng, model)
			existingComponents := model[eid]
			missing := slices.DeleteFunc(slices.Clone(allComponentNames), func(name string) bool {
				_, exists := existingComponents[name]
				return exists
			})
			if len(missing) == 0 {
				continue // Entity has all components
			}
			c := randComponentByName(prng, missing[prng.IntN(len(missing))])
			aidBefore, _ := impl.entityArch.get(eid)

			setComponentAbstract(t, impl, eid, c)
			model[eid][c.Name()] = c

			// Property: archetype should change (move to new archetype).
			aidAfter, exists := impl.entityArch.get(eid)
			assert.True(t, exists, "setComponentMove(%d) entity should exist", eid)
			assert.NotEqual(t, aidBefore, aidAfter, "setComponentMove(%d) archetype should change", eid)

		case ws_compRemove:
			if len(model) == 0 {
				continue
			}
			eid := testutils.RandMapKey(prng, model)
			c := randComponentByName(prng, allComponentNames[prng.IntN(len(allComponentNames))])

			removeComponentAbstract(t, impl, eid, c.Name())
			delete(model[eid], c.Name())

			// Property: get should not return the removed component.
			_, ok := getComponentAbstract(t, impl, eid, c.Name())
			assert.False(t, ok, "removeComponent(%d, %s) then get should not exist", eid, c.Name())

		case ws_compGet:
			if len(model) == 0 {
				continue
			}
			eid := testutils.RandMapKey(prng, model)
			c := randComponentByName(prng, allComponentNames[prng.IntN(len(allComponentNames))])
			name := c.Name()

			implValue, implOk := getComponentAbstract(t, impl, eid, name)
			modelValue, modelOk := model[eid][name]

			assert.Equal(t, modelOk, implOk, "getComponent(%d, %s) existence mismatch", eid, name)
			if modelOk {
				assert.Equal(t, modelValue, implValue, "getComponent(%d, %s) value mismatch", eid, name)
			}

		default:
			panic("unreachable")
		}
	}

	// Property: every entity in entityArch maps to a valid archetype that contains that entity.
	for i, idx := range impl.entityArch {
		if idx == sparseTombstone {
			continue
		}
		eid := EntityID(i)

		aid, exists := impl.entityArch.get(eid)
		assert.True(t, exists, "entity %d in entities but not in entityArch", eid)
		assert.Less(t, aid, len(impl.archetypes), "entity %d maps to invalid archetype %d", eid, aid)

		arch := impl.archetypes[aid]
		row, exists := arch.rows.get(eid)
		assert.True(t, exists, "entity %d in entityArch but not in archetype %d", eid, aid)
		if exists {
			assert.Equal(t, eid, arch.entities[row], "bijection broken: arch.entities[%d] != %d", row, eid)
		}
	}

	// Property: no duplicate entities across all archetypes.
	seenEntities := make(map[EntityID]archetypeID)
	for _, arch := range impl.archetypes {
		for _, eid := range arch.entities {
			if prevAid, seen := seenEntities[eid]; seen {
				t.Errorf("entity %d exists in both archetype %d and %d", eid, prevAid, arch.id)
			}
			seenEntities[eid] = arch.id
		}
	}

	// Final state check: verify all entities and components match between impl and model.
	for eid, modelComponents := range model {
		_, exists := impl.entityArch.get(eid)
		assert.True(t, exists, "entity %d in model but not in impl", eid)

		for name, modelValue := range modelComponents {
			implValue, ok := getComponentAbstract(t, impl, eid, name)
			assert.True(t, ok, "entity %d component %s in model but not in impl", eid, name)
			assert.Equal(t, modelValue, implValue, "entity %d component %s value mismatch", eid, name)
		}

		// Check that impl has no extra components beyond what model has.
		for _, name := range allComponentNames {
			_, implHas := getComponentAbstract(t, impl, eid, name)
			_, modelHas := modelComponents[name]
			assert.Equal(t, modelHas, implHas, "entity %d component %s existence mismatch", eid, name)
		}
	}
}

type worldStateOp uint8

const (
	ws_entityNew     worldStateOp = 16
	ws_entityRemove  worldStateOp = 14
	ws_compSetUpdate worldStateOp = 15 // Update existing component (no archetype change)
	ws_compSetMove   worldStateOp = 30 // Set new component (triggers archetype move)
	ws_compRemove    worldStateOp = 20
	ws_compGet       worldStateOp = 5
)

var worldStateOps = []worldStateOp{
	ws_entityNew, ws_entityRemove, ws_compSetUpdate, ws_compSetMove, ws_compRemove, ws_compGet,
}

func getComponentAbstract(t *testing.T, impl *worldState, eid EntityID, name string) (Component, bool) {
	var res Component
	var err error

	switch name {
	case testutils.ComponentA{}.Name():
		res, err = getComponent[testutils.ComponentA](impl, eid)
	case testutils.ComponentB{}.Name():
		res, err = getComponent[testutils.ComponentB](impl, eid)
	case testutils.ComponentC{}.Name():
		res, err = getComponent[testutils.ComponentC](impl, eid)
	default:
		panic("unreachable")
	}

	// Possible errors here are:
	// - ErrEntityNotFound: entity doesn't exist
	// - "entity doesn't contain component": entity exists but lacks this component
	// - ErrComponentNotFound (unregistered component) can't happen since all components are
	// So we can safely return nil here as these errors are recoverable.
	if err != nil {
		return nil, false
	}
	return res, true
}

func setComponentAbstract(t *testing.T, impl *worldState, eid EntityID, c Component) {
	t.Helper()
	var err error

	name := c.Name()
	switch name {
	case testutils.ComponentA{}.Name():
		err = setComponent[testutils.ComponentA](impl, eid, c.(testutils.ComponentA))
	case testutils.ComponentB{}.Name():
		err = setComponent[testutils.ComponentB](impl, eid, c.(testutils.ComponentB))
	case testutils.ComponentC{}.Name():
		err = setComponent[testutils.ComponentC](impl, eid, c.(testutils.ComponentC))
	default:
		panic("unreachable")
	}
	require.NoError(t, err)
}

func removeComponentAbstract(t *testing.T, impl *worldState, eid EntityID, name string) {
	t.Helper()
	var err error
	switch name {
	case testutils.ComponentA{}.Name():
		err = removeComponent[testutils.ComponentA](impl, eid)
	case testutils.ComponentB{}.Name():
		err = removeComponent[testutils.ComponentB](impl, eid)
	case testutils.ComponentC{}.Name():
		err = removeComponent[testutils.ComponentC](impl, eid)
	default:
		panic("unreachable")
	}
	require.NoError(t, err)
}

var allComponentNames = []string{
	testutils.ComponentA{}.Name(), testutils.ComponentB{}.Name(), testutils.ComponentC{}.Name(),
}

func randComponentByName(prng *rand.Rand, name string) Component {
	switch name {
	case testutils.ComponentA{}.Name():
		return testutils.ComponentA{X: prng.Float64(), Y: prng.Float64(), Z: prng.Float64()}
	case testutils.ComponentB{}.Name():
		return testutils.ComponentB{ID: prng.Uint64(), Label: "test", Enabled: prng.Float64() < 0.5}
	case testutils.ComponentC{}.Name():
		return testutils.ComponentC{Counter: uint16(prng.IntN(65536))}
	default:
		panic("unknown component: " + name)
	}
}

func newTestWorldState(t *testing.T) *worldState {
	t.Helper()
	ws := newWorldState()
	_, err := registerComponent[testutils.ComponentA](ws)
	require.NoError(t, err)
	_, err = registerComponent[testutils.ComponentB](ws)
	require.NoError(t, err)
	_, err = registerComponent[testutils.ComponentC](ws)
	require.NoError(t, err)
	return ws
}

// -------------------------------------------------------------------------------------------------
// Serialization smoke test
//
// We don't extensively test serialize/deserialize because:
// 1. The implementation delegates to archetype and sparse set serialization (tested separately).
// 2. The remaining logic is straightforward type conversion loops.
// 3. Heavy property-based testing would mostly exercise the underlying serialization, not our code.
// -------------------------------------------------------------------------------------------------

func TestWorldState_SerializationSmoke(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const entityMax = 1000

	ws1 := newTestWorldState(t)

	// Create random entities with random components.
	entityCount := prng.IntN(entityMax)
	for range entityCount {
		eid := ws1.newEntity()

		// Randomly add 0-3 components.
		numComponents := prng.IntN(4)
		names := slices.Clone(allComponentNames)
		prng.Shuffle(len(names), func(i, j int) { names[i], names[j] = names[j], names[i] })
		for i := range numComponents {
			c := randComponentByName(prng, names[i])
			setComponentAbstract(t, ws1, eid, c)
		}
	}

	// Remove some entities to populate free list.
	removeCount := prng.IntN(entityCount / 4)
	for range removeCount {
		eid := EntityID(prng.IntN(entityCount))
		ws1.removeEntity(eid)
	}

	pb, err := ws1.toProto()
	require.NoError(t, err)

	ws2 := newTestWorldState(t)
	err = ws2.fromProto(pb)
	require.NoError(t, err)

	// Property: deserialize(serialize(x)) == x.
	assert.Equal(t, ws1.nextID, ws2.nextID)
	assert.Equal(t, ws1.free, ws2.free)
	assert.Equal(t, sparseSet(ws1.entityArch), sparseSet(ws2.entityArch))

	assert.Len(t, ws2.archetypes, len(ws1.archetypes))
	for i := range ws1.archetypes {
		arch1 := ws1.archetypes[i]
		arch2 := ws2.archetypes[i]

		assert.Equal(t, arch1.id, arch2.id)
		assert.Equal(t, arch1.components.ToBytes(), arch2.components.ToBytes())
		assert.Equal(t, arch1.entities, arch2.entities)
		assert.Equal(t, sparseSet(arch1.rows), sparseSet(arch2.rows))

		assert.Len(t, arch2.columns, len(arch1.columns))
		for j := range arch1.columns {
			assert.Equal(t, arch1.columns[j].len(), arch2.columns[j].len())
			for k := range arch1.columns[j].len() {
				assert.Equal(t, arch1.columns[j].getAbstract(k), arch2.columns[j].getAbstract(k))
			}
		}
	}
}
