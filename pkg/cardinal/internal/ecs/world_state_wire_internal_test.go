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

func (c wirePos) MarshalWire() ([]byte, error) { return c.AppendWire(nil), nil }

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

// newWireTestWorld registers a direct component (wirePos) and a fallback component
// (testutils.SimpleComponent, gob-encoded, no SizeWire) so every encoder path runs.
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
	assert.Equal(t, []string{"simple_component", "wire_pos"}, decoded.GetComponents(), "name table sorted by name")
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

	// The free list is the gaps, ascending; the hole left by nothing here means only IDs >= nextID
	// are free — both worlds must agree.
	assert.Equal(t, ws.free, restored.free)
	assert.Equal(t, ws.nextID, restored.nextID)
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

// TestSnapshotWireRejectsBadInput: restore refuses malformed files before or during the rebuild,
// never silently.
func TestSnapshotWireRejectsBadInput(t *testing.T) {
	t.Parallel()

	cases := map[string]*cardinalv1.WorldState{
		"unknown component": {
			NextId:     1,
			Components: []string{"nope"},
			Entities:   []*cardinalv1.Entity{{Components: []uint32{0}, Payloads: [][]byte{{}}}},
		},
		"unsorted name table": {
			NextId:     1,
			Components: []string{"wire_pos", "a_simple"},
		},
		"entities not ascending": {
			NextId: 3,
			Entities: []*cardinalv1.Entity{
				{Id: 2}, {Id: 1},
			},
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
			require.Error(t, ws.fromProto(pb))
		})
	}
}

// TestSnapshotWireAllocations measures the hot path. Direct components must not allocate at all;
// the one remaining cost is the fallback component's MarshalWire, which disappears per type as the
// generator's SizeWire/AppendWire land.
func TestSnapshotWireAllocations(t *testing.T) {
	// Not parallel: testing.AllocsPerRun panics in parallel tests.
	ws, posID, _ := newWireTestWorld(t)
	var onlyPos bitmap.Bitmap
	onlyPos.Set(posID)
	for i := range 100 {
		eid := ws.newEntityWithArchetype(onlyPos)
		require.NoError(t, setComponent(ws, eid, wirePos{X: float64(i), Y: 1}))
	}

	// Warm the scratch (sortedCIDs, tableIdx) and learn the buffer size.
	size := ws.wireBodySize()
	buf := make([]byte, 0, size)

	allocs := testing.AllocsPerRun(50, func() {
		ws.wireBodySize()
		buf = ws.appendWireBody(buf[:0])
	})
	assert.Zero(t, allocs, "encoding direct components must not allocate")
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
