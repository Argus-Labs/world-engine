package ecs

import (
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
	"github.com/kelindar/bitmap"
	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
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
		res, err = impl.getComponent[testutils.ComponentA](eid)
	case testutils.ComponentB{}.Name():
		res, err = impl.getComponent[testutils.ComponentB](eid)
	case testutils.ComponentC{}.Name():
		res, err = impl.getComponent[testutils.ComponentC](eid)
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
		err = impl.setComponent(eid, c.(testutils.ComponentA))
	case testutils.ComponentB{}.Name():
		err = impl.setComponent(eid, c.(testutils.ComponentB))
	case testutils.ComponentC{}.Name():
		err = impl.setComponent(eid, c.(testutils.ComponentC))
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
		err = impl.removeComponent[testutils.ComponentA](eid)
	case testutils.ComponentB{}.Name():
		err = impl.removeComponent[testutils.ComponentB](eid)
	case testutils.ComponentC{}.Name():
		err = impl.removeComponent[testutils.ComponentC](eid)
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
// by the entity id generator test, but under concurrent operations. Run with -race to detect data
// races (go test -race). We coordinate goroutine start with an explicit barrier to maximize
// contention in the entity ID allocation/recycling path. We only test entity operations
// concurrently because component operations (get/set/remove) are not concurrent-safe. The system
// scheduler ensures operations on the same component type are never done concurrently in multiple
// systems.
// -------------------------------------------------------------------------------------------------

func TestWorldState_EntityFuzzConcurrent(t *testing.T) {
	t.Parallel()

	const (
		numGoroutines     = 10
		opsPerGoroutine   = 1000
		createRemoveRatio = 0.6
	)

	ws := newTestWorldState(t)

	var createCount, removeCount atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})

	for range numGoroutines {
		wg.Go(func() {
			// Initialize prng in each goroutine separately because rand/v2.Rand isn't concurrent-safe.
			prng := testutils.NewRand(t)
			<-start

			for range opsPerGoroutine {
				if prng.Float64() < createRemoveRatio {
					ws.newEntity()
					createCount.Add(1)
				} else {
					// Read nextID under lock to avoid racing with concurrent newEntity calls.
					ws.mu.Lock()
					nextID := ws.nextID
					ws.mu.Unlock()
					if nextID > 0 {
						eid := EntityID(prng.IntN(int(nextID)))
						ws.removeEntity(eid)
					}
					// Increment regardless of whether we removed any entities.
					removeCount.Add(1)
				}
			}
		})
	}

	// Release all workers simultaneously to maximize overlapping entity operations.
	close(start)
	wg.Wait()

	// Property: total operations equals expected count.
	totalOps := createCount.Load() + removeCount.Load()
	assert.Equal(t, int64(numGoroutines*opsPerGoroutine), totalOps,
		"total operations mismatch: creates=%d, removes=%d", createCount.Load(), removeCount.Load())

	assertEntityIDInvariants(t, ws)
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
	_, err := w.RegisterComponent[testutils.ComponentA]()
	require.NoError(t, err)
	_, err = w.RegisterComponent[testutils.ComponentB]()
	require.NoError(t, err)
	_, err = w.RegisterComponent[testutils.ComponentC]()
	require.NoError(t, err)
	return w.state
}

// -------------------------------------------------------------------------------------------------
// Serialization
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

	size, err := ws1.wireBodySize()
	require.NoError(t, err)
	data, err := ws1.appendWireBody(make([]byte, 0, size))
	require.NoError(t, err)
	require.Len(t, data, size)

	var pb cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(data, &pb))

	ws2 := newTestWorldState(t)
	require.NoError(t, ws2.fromProto(&pb))

	// Property: deserialize(serialize(x)) describes the same world as x, and re-serializing the
	// rebuilt world reproduces the same bytes. Runtime layout (archetype ids, rows, entityArch) is
	// deliberately not compared: the file no longer records it, and a rebuild is free to produce a
	// different but equivalent layout.
	assertWorldStateEqual(t, ws1, ws2)

	size2, err := ws2.wireBodySize()
	require.NoError(t, err)
	data2, err := ws2.appendWireBody(make([]byte, 0, size2))
	require.NoError(t, err)
	assert.Equal(t, data, data2, "snapshot -> restore -> snapshot must be byte-stable")
}

// assertWorldStateEqual checks that two worldStates describe the same world: the same entities,
// each with the same components and values.
func assertWorldStateEqual(t *testing.T, ws1, ws2 *worldState) {
	t.Helper()

	assert.Equal(t, ws1.nextID, ws2.nextID)

	// free is a min-heap, so two worlds holding the same ids can hold them in different array order.
	// The guarantee is pop order, asserted in TestSnapshotWireFreeListSurvives; here it is the set.
	assert.ElementsMatch(t, ws1.free, ws2.free)

	live := func(ws *worldState) map[EntityID]map[string]Component {
		out := make(map[EntityID]map[string]Component)
		for _, arch := range ws.archetypes {
			for _, eid := range arch.entities {
				row, ok := arch.rows.get(eid)
				require.True(t, ok)
				comps := make(map[string]Component, len(arch.columns))
				for _, col := range arch.columns {
					comps[col.name()] = col.getAbstract(row)
				}
				out[eid] = comps
			}
		}
		return out
	}
	assert.Equal(t, live(ws1), live(ws2))
}

// wirePos is a hand-written stand-in for a generated value-shaped component: it carries the
// SizeWire/AppendWire pair the generator emits, encoding {double x = 1; double y = 2}.
type wirePos struct {
	X, Y float64
}

func (wirePos) Name() string { return "wire_pos" }

func (c wirePos) SizeWire() int {
	n := 0
	if c.X != 0 {
		n += protowire.SizeTag(1) + 8
	}
	if c.Y != 0 {
		n += protowire.SizeTag(2) + 8
	}
	return n
}

func (c wirePos) AppendWire(b []byte) []byte {
	if c.X != 0 {
		b = protowire.AppendTag(b, 1, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, math.Float64bits(c.X))
	}
	if c.Y != 0 {
		b = protowire.AppendTag(b, 2, protowire.Fixed64Type)
		b = protowire.AppendFixed64(b, math.Float64bits(c.Y))
	}
	return b
}

func (c wirePos) MarshalWire() []byte { return c.AppendWire(nil) }

func (c wirePos) UnmarshalWire(data []byte) (any, error) {
	var out wirePos
	for len(data) > 0 {
		num, typ, n := protowire.ConsumeTag(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		if typ != protowire.Fixed64Type {
			return nil, protowire.ParseError(-1)
		}
		v, n := protowire.ConsumeFixed64(data)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		data = data[n:]
		switch num { //nolint:exhaustive // only fields 1 and 2 exist
		case 1:
			out.X = math.Float64frombits(v)
		case 2:
			out.Y = math.Float64frombits(v)
		}
	}
	return out, nil
}

// newWireTestWorld registers two components that encode in completely different ways, so the tests
// below prove the snapshot encoder is agnostic to how any given component encodes itself.
func newWireTestWorld(t *testing.T) (*worldState, ComponentID, ComponentID) {
	t.Helper()
	ws := newWorldState()

	posID, err := ws.components.register("wire_pos", newColumnFactory[wirePos]())
	require.NoError(t, err)
	simpleID, err := ws.components.register(
		testutils.SimpleComponent{}.Name(), newColumnFactory[testutils.SimpleComponent]())
	require.NoError(t, err)
	return ws, posID, simpleID
}

func encodeWorld(t *testing.T, ws *worldState) []byte {
	t.Helper()
	size, err := ws.wireBodySize()
	require.NoError(t, err)
	buf, err := ws.appendWireBody(make([]byte, 0, size))
	require.NoError(t, err)
	require.Len(t, buf, size, "append must write exactly what the size pass computed")
	return buf
}

// TestSnapshotWireCanonical: the streamed bytes must be exactly what proto.Marshal produces for the
// same message. Decoding and re-marshaling with the generated code is the oracle: if the bytes
// round-trip to themselves, the hand encoder agrees with protobuf on every field, order, and varint.
func TestSnapshotWireCanonical(t *testing.T) {
	t.Parallel()
	ws, posID, simpleID := newWireTestWorld(t)

	// Entities across archetypes, in deliberately shuffled creation order, one destroyed to leave a
	// gap, one with zero components.
	var both bitmap.Bitmap
	both.Set(posID)
	both.Set(simpleID)
	var onlyPos bitmap.Bitmap
	onlyPos.Set(posID)

	e0 := ws.newEntityWithArchetype(both)
	e1 := ws.newEntityWithArchetype(onlyPos)
	e2 := ws.newEntity() // zero components
	e3 := ws.newEntityWithArchetype(both)
	require.NoError(t, setComponent(ws, e0, wirePos{X: 1.5, Y: -2}))
	require.NoError(t, setComponent(ws, e0, testutils.SimpleComponent{Value: 7}))
	require.NoError(t, setComponent(ws, e1, wirePos{X: 42}))
	require.NoError(t, setComponent(ws, e3, wirePos{Y: 9}))
	require.NoError(t, setComponent(ws, e3, testutils.SimpleComponent{Value: -1}))
	require.True(t, ws.removeEntity(e1)) // leaves a free-list hole
	_ = e2

	data := encodeWorld(t, ws)

	var decoded cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(data, &decoded))

	// Semantic shape.
	assert.Equal(t, uint32(4), decoded.GetNextId())
	assert.Equal(t, []string{"wire_pos", "simple_component"}, decoded.GetComponents(),
		"name table in registration order")
	require.Len(t, decoded.GetEntities(), 3, "e1 destroyed; e0, e2, e3 alive")
	assert.Equal(t, uint32(0), decoded.GetEntities()[0].GetId())
	assert.Equal(t, uint32(2), decoded.GetEntities()[1].GetId())
	assert.Equal(t, uint32(3), decoded.GetEntities()[2].GetId())
	assert.Empty(t, decoded.GetEntities()[1].GetComponents(), "zero-component entity is present and empty")
	assert.Equal(t, []uint32{0, 1}, decoded.GetEntities()[0].GetComponents(), "indices ascend with the table")

	// Byte canonicality.
	remarshaled, err := proto.MarshalOptions{Deterministic: true}.Marshal(&decoded)
	require.NoError(t, err)
	assert.Equal(t, remarshaled, data, "streamed bytes must be exactly proto.Marshal's encoding")
}

// TestSnapshotWireRoundTrip: encode -> restore into a fresh world -> encode again must be
// byte-identical, and the restored world must serve the same component values.
func TestSnapshotWireRoundTrip(t *testing.T) {
	t.Parallel()
	ws, posID, simpleID := newWireTestWorld(t)

	var both bitmap.Bitmap
	both.Set(posID)
	both.Set(simpleID)
	e0 := ws.newEntityWithArchetype(both)
	_ = ws.newEntity()
	require.NoError(t, setComponent(ws, e0, wirePos{X: 3, Y: 4}))
	require.NoError(t, setComponent(ws, e0, testutils.SimpleComponent{Value: 11}))

	data := encodeWorld(t, ws)
	var pb cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(data, &pb))

	restored, _, _ := newWireTestWorld(t)
	require.NoError(t, restored.fromProto(&pb))

	// Same bytes out of the rebuilt world.
	assert.Equal(t, data, encodeWorld(t, restored), "snapshot -> restore -> snapshot must be byte-stable")

	// Same values through the rebuilt lookup paths.
	pos, err := getComponent[wirePos](restored, e0)
	require.NoError(t, err)
	assert.Equal(t, wirePos{X: 3, Y: 4}, pos)
	simple, err := getComponent[testutils.SimpleComponent](restored, e0)
	require.NoError(t, err)
	assert.Equal(t, testutils.SimpleComponent{Value: 11}, simple)

	assert.ElementsMatch(t, ws.free, restored.free)
	assert.Equal(t, ws.nextID, restored.nextID)
}

// TestSnapshotWireFreeListSurvives: ids freed out of order must still be reused in the same order
// after a restore. The snapshot stores only the gaps, so the allocator has to be order-independent.
func TestSnapshotWireFreeListSurvives(t *testing.T) {
	t.Parallel()
	ws, _, _ := newWireTestWorld(t)
	for range 4 { // ids 0..3
		_ = ws.newEntity()
	}
	require.True(t, ws.removeEntity(2))
	require.True(t, ws.removeEntity(0)) // freed after 2, but lower

	data := encodeWorld(t, ws)
	var pb cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(data, &pb))
	restored, _, _ := newWireTestWorld(t)
	require.NoError(t, restored.fromProto(&pb))

	assert.ElementsMatch(t, ws.free, restored.free)
	// The real guarantee: both worlds hand out the same ids from here on.
	assert.Equal(t, ws.newEntity(), restored.newEntity())
	assert.Equal(t, ws.newEntity(), restored.newEntity())
	assert.Equal(t, ws.newEntity(), restored.newEntity())
}

// TestSnapshotWireDeterministic: two worlds reaching the same state through different operation
// orders must produce identical bytes.
func TestSnapshotWireDeterministic(t *testing.T) {
	t.Parallel()

	build := func(reversed bool) []byte {
		ws, posID, simpleID := newWireTestWorld(t)
		var both bitmap.Bitmap
		both.Set(posID)
		both.Set(simpleID)
		var onlyPos bitmap.Bitmap
		onlyPos.Set(posID)

		if reversed {
			// Same end state, built via moves instead of direct archetype creation.
			a := ws.newEntity()
			b := ws.newEntity()
			require.NoError(t, setComponent(ws, b, wirePos{X: 2}))
			require.NoError(t, setComponent(ws, a, testutils.SimpleComponent{Value: 5}))
			require.NoError(t, setComponent(ws, a, wirePos{X: 1}))
		} else {
			a := ws.newEntityWithArchetype(both)
			b := ws.newEntityWithArchetype(onlyPos)
			require.NoError(t, setComponent(ws, a, wirePos{X: 1}))
			require.NoError(t, setComponent(ws, a, testutils.SimpleComponent{Value: 5}))
			require.NoError(t, setComponent(ws, b, wirePos{X: 2}))
		}
		return encodeWorld(t, ws)
	}

	assert.Equal(t, build(false), build(true),
		"identical worlds with different archetype histories must encode identically")
}

// TestSnapshotWireDroppedComponent: the writer emits every registered component name, so dropping a
// component type must not make its own prior snapshots unreadable. An unknown name is only fatal if
// an entity actually holds it.
func TestSnapshotWireDroppedComponent(t *testing.T) {
	t.Parallel()

	old, posID, _ := newWireTestWorld(t) // registers wire_pos and simple_component
	var only bitmap.Bitmap
	only.Set(posID)
	e := old.newEntityWithArchetype(only) // uses wire_pos only
	require.NoError(t, setComponent(old, e, wirePos{X: 1, Y: 2}))

	var pb cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(encodeWorld(t, old), &pb))
	require.Contains(t, pb.GetComponents(), testutils.SimpleComponent{}.Name(),
		"the writer should emit every registered name, used or not")

	// A build that dropped the unused component.
	dropped := newWorldState()
	_, err := dropped.components.register("wire_pos", newColumnFactory[wirePos]())
	require.NoError(t, err)

	require.NoError(t, dropped.fromProto(&pb), "an unused dropped component must not block restore")
	got, err := getComponent[wirePos](dropped, e)
	require.NoError(t, err)
	assert.Equal(t, wirePos{X: 1, Y: 2}, got)
}

// And the other half: dropping a component an entity DOES hold still fails, naming it.
func TestSnapshotWireDroppedComponentInUse(t *testing.T) {
	t.Parallel()

	old, posID, simpleID := newWireTestWorld(t)
	var both bitmap.Bitmap
	both.Set(posID)
	both.Set(simpleID)
	e := old.newEntityWithArchetype(both)
	require.NoError(t, setComponent(old, e, wirePos{X: 1}))
	require.NoError(t, setComponent(old, e, testutils.SimpleComponent{Value: 5}))

	var pb cardinalv1.WorldState
	require.NoError(t, proto.Unmarshal(encodeWorld(t, old), &pb))

	dropped := newWorldState()
	_, err := dropped.components.register("wire_pos", newColumnFactory[wirePos]())
	require.NoError(t, err)

	err = dropped.fromProto(&pb)
	require.Error(t, err)
	assert.Contains(t, err.Error(), testutils.SimpleComponent{}.Name())
}

// TestSnapshotWireRejectsBadInput: restore refuses malformed files, and leaves the live world
// exactly as it was — the rebuild is committed only once the whole file is accepted.
func TestSnapshotWireRejectsBadInput(t *testing.T) {
	t.Parallel()

	cases := map[string]*cardinalv1.WorldState{
		"nil message": nil,
		"unknown component": {
			NextId:     1,
			Components: []string{"nope"},
			Entities:   []*cardinalv1.Entity{{Components: []uint32{0}, Payloads: [][]byte{{}}}},
		},
		"duplicate name in table": {
			NextId:     1,
			Components: []string{"wire_pos", "wire_pos"},
		},
		"entities not ascending": {
			NextId: 3,
			Entities: []*cardinalv1.Entity{
				{Id: 2}, {Id: 1},
			},
		},
		"next_id implies an unbounded free list": {
			NextId: math.MaxUint32, // 5 bytes of input, 4.3 billion ids
		},
		// The same amplification through one entity at a high id: the gaps below it size both the
		// free heap and the entityArch sparse index.
		"one entity at a high id": {
			NextId:   math.MaxUint32,
			Entities: []*cardinalv1.Entity{{Id: math.MaxUint32 - 1}},
		},
		"more entities than next_id allows": {
			NextId:   1,
			Entities: []*cardinalv1.Entity{{Id: 0}, {Id: 0}},
		},
		"entity above next_id": {
			NextId:   1,
			Entities: []*cardinalv1.Entity{{Id: 5}},
		},
		"index outside table": {
			NextId:     1,
			Components: []string{"wire_pos"},
			Entities:   []*cardinalv1.Entity{{Components: []uint32{3}, Payloads: [][]byte{{}}}},
		},
		"payload count mismatch": {
			NextId:     1,
			Components: []string{"wire_pos"},
			Entities:   []*cardinalv1.Entity{{Components: []uint32{0}, Payloads: [][]byte{}}},
		},
	}

	for name, pb := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ws, _, _ := newWireTestWorld(t)
			e := ws.newEntity()

			require.Error(t, ws.fromProto(pb))

			_, live := ws.entityArch.get(e)
			assert.True(t, live, "a refused restore must leave the world untouched")
			assert.Equal(t, EntityID(1), ws.nextID)
		})
	}
}

// TestSnapshotWireAllocations measures the hot path, which must not allocate at all.
func TestSnapshotWireAllocations(t *testing.T) {
	// Not parallel: testing.AllocsPerRun panics in parallel tests.
	ws, posID, _ := newWireTestWorld(t)
	var onlyPos bitmap.Bitmap
	onlyPos.Set(posID)
	for i := range 100 {
		eid := ws.newEntityWithArchetype(onlyPos)
		require.NoError(t, setComponent(ws, eid, wirePos{X: float64(i), Y: 1}))
	}

	// Learn the buffer size, so the measured runs below append into a buffer that never grows.
	size, err := ws.wireBodySize()
	require.NoError(t, err)
	buf := make([]byte, 0, size)

	allocs := testing.AllocsPerRun(50, func() {
		if _, err = ws.wireBodySize(); err != nil {
			return
		}
		buf, err = ws.appendWireBody(buf[:0])
	})
	require.NoError(t, err)
	assert.Zero(t, allocs, "the size and append passes must not allocate")
}

// TestSnapshotWireCorruptionIsAnError: an entity whose archetype has no row for it is an error in
// both passes, not a panic. The world keeps running without a snapshot.
func TestSnapshotWireCorruptionIsAnError(t *testing.T) {
	t.Parallel()
	ws, posID, _ := newWireTestWorld(t)
	var onlyPos bitmap.Bitmap
	onlyPos.Set(posID)
	e := ws.newEntityWithArchetype(onlyPos)

	size, err := ws.wireBodySize()
	require.NoError(t, err)

	// Corrupt the world: the archetype forgets the entity but the index still points at it.
	aid, ok := ws.entityArch.get(e)
	require.True(t, ok)
	ws.archetypes[aid].removeEntity(e)

	_, err = ws.appendWireBody(make([]byte, 0, size))
	require.Error(t, err)
	_, err = ws.wireBodySize()
	require.Error(t, err)
}

// TestSnapshotWireDivergenceIsAnError: a world that changes between the two passes is an error,
// not a panic.
func TestSnapshotWireDivergenceIsAnError(t *testing.T) {
	t.Parallel()
	ws, _, _ := newWireTestWorld(t)
	_ = ws.newEntity()

	size, err := ws.wireBodySize()
	require.NoError(t, err)
	_ = ws.newEntity() // mutate between the passes

	_, err = ws.appendWireBody(make([]byte, 0, size))
	require.Error(t, err)
}

// TestSnapshotWireFieldCoverage guards the one proto change the canonical test cannot see.
//
// The hand encoder writes field numbers as literals, so it agrees with the generated code only by
// convention. Renumbering or retyping a field breaks TestSnapshotWireCanonical, because that test
// decodes the streamed bytes with the generated code and checks the values. Adding a field does
// not: the encoder simply never writes it, decode yields the zero value, and every assertion still
// passes while the new field is silently missing from every snapshot.
//
// This test fails instead. When it does, teach the encoder the new field, then update the list.
func TestSnapshotWireFieldCoverage(t *testing.T) {
	t.Parallel()

	encoded := map[string][]protoreflect.FieldNumber{
		"WorldState": {1, 2, 3}, // next_id, components, entities
		"Entity":     {1, 2, 3}, // id, components, payloads
	}
	descriptors := []protoreflect.MessageDescriptor{
		(&cardinalv1.WorldState{}).ProtoReflect().Descriptor(),
		(&cardinalv1.Entity{}).ProtoReflect().Descriptor(),
	}

	for _, desc := range descriptors {
		name := string(desc.Name())
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			fields := desc.Fields()
			actual := make([]protoreflect.FieldNumber, 0, fields.Len())
			for i := range fields.Len() {
				actual = append(actual, fields.Get(i).Number())
			}
			assert.Equal(t, encoded[name], actual,
				"%s fields changed: world_state.go writes these numbers as literals and must "+
					"be updated to match", name)
		})
	}
}
