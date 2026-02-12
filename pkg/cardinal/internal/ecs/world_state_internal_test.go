package ecs

import (
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -------------------------------------------------------------------------------------------------
// Model-based fuzzing world state operations
// -------------------------------------------------------------------------------------------------
// This test verifies the worldState implementation correctness by applying random sequences of
// operations and comparing it against a Go map of map[EntityID]map[string]any as the model.
// We also verify structural invariants: entity-archetype bijection and global entity uniqueness.
// -------------------------------------------------------------------------------------------------

func TestWorldState_ModelFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax          = 1 << 15 // 32_768 iterations
		opEntityNew     = "entityNew"
		opEntityRemove  = "entityRemove"
		opCompSetUpdate = "compSetUpdate"
		opCompSetMove   = "compSetMove"
		opCompRemove    = "compRemove"
		opCompGet       = "compGet"
	)

	// Randomize operation weights.
	operations := []string{opEntityNew, opEntityRemove, opCompSetUpdate, opCompSetMove, opCompRemove, opCompGet}
	weights := testutils.RandOpWeights(prng, operations)

	impl := newTestWorldState(t)
	model := make(map[EntityID]map[string]any)

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opEntityNew:
			eid := impl.newEntity()
			model[eid] = make(map[string]any)

			// Property: new entity should exist in entityArch.
			_, exists := impl.entityArch.get(eid)
			assert.True(t, exists, "newEntity(%d) should exist in entityArch", eid)

		case opEntityRemove:
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

		case opCompSetUpdate:
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
			assert.True(t, ok, "entity %d should exist before update", eid)

			setComponentAbstract(t, impl, eid, c)
			model[eid][c.Name()] = c

			// Property: archetype should NOT change (update in place).
			aidAfter, exists := impl.entityArch.get(eid)
			assert.True(t, exists, "setComponentUpdate(%d) entity should exist", eid)
			assert.Equal(t, aidBefore, aidAfter, "setComponentUpdate(%d) archetype should not change", eid)

		case opCompSetMove:
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

		case opCompRemove:
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

		case opCompGet:
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

	// We can ignore these errors, as the tests randomly select eid and component:
	// - ErrEntityNotFound: entity doesn't exist
	// - "entity doesn't contain component": entity exists but lacks this component
	if err != nil {
		assert.False(t, eris.Is(err, ErrComponentNotFound), "component isn't registered")
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
		err = setComponent(impl, eid, c.(testutils.ComponentA))
	case testutils.ComponentB{}.Name():
		err = setComponent(impl, eid, c.(testutils.ComponentB))
	case testutils.ComponentC{}.Name():
		err = setComponent(impl, eid, c.(testutils.ComponentC))
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

// -------------------------------------------------------------------------------------------------
// Entity ID generator fuzz
// -------------------------------------------------------------------------------------------------
// This test runs random sequences of newEntity/removeEntity operations and verifies that the
// entity ID generator invariants hold: nextID monotonicity, live/free disjointness, all IDs
// bounded by nextID, and no duplicate live entities.
// -------------------------------------------------------------------------------------------------

func TestWorldState_EntityFuzz(t *testing.T) {
	t.Parallel()
	prng := testutils.NewRand(t)

	const (
		opsMax   = 1 << 15 // 32_768 iterations
		opCreate = "create"
		opRemove = "remove"
	)

	// Randomize operation weights.
	operations := []string{opCreate, opRemove}
	weights := testutils.RandOpWeights(prng, operations)

	impl := newTestWorldState(t)
	prevNextID := impl.nextID

	for range opsMax {
		op := testutils.RandWeightedOp(prng, weights)
		switch op {
		case opCreate:
			impl.newEntity()

		case opRemove:
			if impl.nextID == 0 {
				continue
			}
			eid := EntityID(prng.IntN(int(impl.nextID)))
			impl.removeEntity(eid) // May return false if already removed.

		default:
			panic("unreachable")
		}

		// Property: nextID is monotonically non-decreasing.
		assert.GreaterOrEqual(t, impl.nextID, prevNextID, "nextID decreased")
		prevNextID = impl.nextID
	}

	assertEntityIDInvariants(t, impl)
}

// -------------------------------------------------------------------------------------------------
// Concurrent entity operations fuzz
// -------------------------------------------------------------------------------------------------
// This test verifies that the newEntity/removeEntity operations maintain the same invariants tested
// by the entity id generator test, but under concurrent operations using Go 1.25 testing/synctest.
// We only test entity operations concurrently because component operations (get/set/remove) are not
// concurrent-safe. The system scheduler ensures operations on the same component type are never
// done concurrently in multiple systems.
// -------------------------------------------------------------------------------------------------

func TestWorldState_EntityFuzzConcurrent(t *testing.T) {
	t.Parallel()

	const (
		numGoroutines     = 10
		opsPerGoroutine   = 1000
		createRemoveRatio = 0.6
	)

	synctest.Test(t, func(t *testing.T) {
		ws := newTestWorldState(t)

		var createCount, removeCount atomic.Int64
		var wg sync.WaitGroup

		for range numGoroutines {
			wg.Go(func() {
				// Initialize prng in each goroutine separately because rand/v2.Rand isn't concurrent-safe.
				prng := testutils.NewRand(t)

				for range opsPerGoroutine {
					if prng.Float64() < createRemoveRatio {
						ws.newEntity()
						createCount.Add(1)
					} else {
						if ws.nextID > 0 {
							eid := EntityID(prng.IntN(int(ws.nextID))) // prng.IntN will fail if nextID is 0
							ws.removeEntity(eid)
						}
						// Increment regardless of whether we removed any entities.
						removeCount.Add(1)
					}
				}
			})
		}
		wg.Wait()

		// Property: total operations equals expected count.
		totalOps := createCount.Load() + removeCount.Load()
		assert.Equal(t, int64(numGoroutines*opsPerGoroutine), totalOps,
			"total operations mismatch: creates=%d, removes=%d", createCount.Load(), removeCount.Load())

		assertEntityIDInvariants(t, ws)
	})
}

// assertEntityIDInvariants checks entity ID generator properties hold. Normally I'd hardcode this
// into the test, but this is used in both tests above so extracting this out.
func assertEntityIDInvariants(t *testing.T, ws *worldState) {
	t.Helper()

	// Property: live and free are disjoint.
	liveSet := make(map[EntityID]struct{})
	for i, idx := range ws.entityArch {
		if idx != sparseTombstone {
			liveSet[EntityID(i)] = struct{}{}
		}
	}
	for _, freeID := range ws.free {
		_, isLive := liveSet[freeID]
		assert.False(t, isLive, "entity %d is both live and free", freeID)
	}

	// Property: all live and free IDs are < nextID.
	for liveID := range liveSet {
		assert.Less(t, liveID, ws.nextID, "live entity %d >= nextID %d", liveID, ws.nextID)
	}
	for _, freeID := range ws.free {
		assert.Less(t, freeID, ws.nextID, "free entity %d >= nextID %d", freeID, ws.nextID)
	}

	// Property: free list has no duplicates.
	freeSet := make(map[EntityID]struct{}, len(ws.free))
	for _, freeID := range ws.free {
		_, exists := freeSet[freeID]
		assert.False(t, exists, "duplicate in free list: %d", freeID)
		freeSet[freeID] = struct{}{}
	}
}

// -------------------------------------------------------------------------------------------------
// Entity ID reuse FIFO test
// -------------------------------------------------------------------------------------------------
// Simple test to verify FIFO property of entity ID reuse.
// -------------------------------------------------------------------------------------------------

func TestWorldState_EntityID_FIFO(t *testing.T) {
	t.Parallel()

	ws := newTestWorldState(t)

	e0 := ws.newEntity()
	e1 := ws.newEntity()
	e2 := ws.newEntity()

	ws.removeEntity(e0)
	ws.removeEntity(e1)
	ws.removeEntity(e2)

	assert.Equal(t, e0, ws.newEntity())
	assert.Equal(t, e1, ws.newEntity())
	assert.Equal(t, e2, ws.newEntity())
}

func newTestWorldState(t *testing.T) *worldState {
	t.Helper()
	w := NewWorld()
	w.OnComponentRegister(func(Component) error { return nil })
	_, err := registerComponent[testutils.ComponentA](w)
	require.NoError(t, err)
	_, err = registerComponent[testutils.ComponentB](w)
	require.NoError(t, err)
	_, err = registerComponent[testutils.ComponentC](w)
	require.NoError(t, err)
	return w.state
}

// -------------------------------------------------------------------------------------------------
// Serialization smoke test
// -------------------------------------------------------------------------------------------------
// We don't extensively test serialize/deserialize because:
// 1. The implementation delegates to archetype serialization (tested separately).
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
	assertWorldStateEqual(t, ws1, ws2)
}

// assertWorldStateEqual checks if two worldStates are structurally equal. This function is
// extracted so it can be reused in serialization tests "above" this layer.
func assertWorldStateEqual(t *testing.T, ws1, ws2 *worldState) {
	t.Helper()

	assert.Equal(t, ws1.nextID, ws2.nextID)
	assert.Equal(t, ws1.free, ws2.free)
	assert.Equal(t, ws1.entityArch, ws2.entityArch)

	assert.Len(t, ws2.archetypes, len(ws1.archetypes))
	for i := range ws1.archetypes {
		assertArchetypeEqual(t, ws1.archetypes[i], ws2.archetypes[i])
	}
}
