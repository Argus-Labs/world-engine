package ecs

import (
	"math"
	"testing"

	"github.com/kelindar/bitmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/argus-labs/world-engine/pkg/testutils"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

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
	size := ws.wireBodySize()
	buf := ws.appendWireBody(make([]byte, 0, size))
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
	size := ws.wireBodySize()
	buf := make([]byte, 0, size)

	allocs := testing.AllocsPerRun(50, func() {
		ws.wireBodySize()
		buf = ws.appendWireBody(buf[:0])
	})
	assert.Zero(t, allocs, "the size and append passes must not allocate")
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
				"%s fields changed: world_state_wire.go writes these numbers as literals and must "+
					"be updated to match", name)
		})
	}
}
