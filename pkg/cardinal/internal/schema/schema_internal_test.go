package schema_test

import (
	"bytes"
	"errors"
	"strconv"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/immutable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

type payload struct {
	ID       uint64         `msgpack:"id"`
	Ints     []int          `msgpack:"ints,omitempty"`
	Metadata map[string]int `msgpack:"metadata,omitempty"`
}

func (payload) Name() string { return "payload" }

func TestSerializeRoundTrip(t *testing.T) {
	t.Parallel()
	want := payload{ID: 18446744073709551615, Ints: []int{-19, 37}, Metadata: map[string]int{"score": 81}}
	data, err := schema.Serialize(want)
	require.NoError(t, err)
	var got payload
	require.NoError(t, schema.Deserialize(data, &got))
	require.Equal(t, want, got)

	// Omitted fields must not retain values from the previous decode.
	data, err = schema.Serialize(payload{ID: 9007199254740993})
	require.NoError(t, err)
	require.NoError(t, schema.Deserialize(data, &got))
	require.Equal(t, payload{ID: 9007199254740993}, got)
}

func TestDeserializeRejectsMalformedInput(t *testing.T) {
	t.Parallel()
	for _, data := range [][]byte{
		nil,
		{0x81, 0xa2, 'i', 'd'}, // Truncated value.
		// Historical regression: BIN8 into []int panics inside msgpack.Unmarshal.
		{0x81, 0xa4, 'i', 'n', 't', 's', 0xc4, 0x03, 'a', 'b', 'c'},
		// Decode one field successfully before failing on the next.
		{0x82, 0xa2, 'i', 'd', 0x07, 0xa4, 'i', 'n', 't', 's', 0xc4, 0x03, 'a', 'b', 'c'},
	} {
		got := payload{ID: 93, Ints: []int{11}, Metadata: map[string]int{"keep": 42}}
		require.ErrorContains(t, schema.Deserialize(data, &got), "failed to deserialize")
		require.Equal(t, payload{ID: 93, Ints: []int{11}, Metadata: map[string]int{"keep": 42}}, got)
	}
	for _, target := range []any{nil, payload{}, (*payload)(nil)} {
		require.ErrorContains(t, schema.Deserialize([]byte{0x80}, target), "non-nil pointer")
	}
}

func TestDeserializeRejectsDeepNesting(t *testing.T) {
	t.Parallel()
	// nested returns a payload nested depth levels deep: the payload map, then an unknown field of
	// one-element arrays, which msgpack would skip by recursing once per level.
	nested := func(depth int) []byte {
		data := []byte{0x82, 0xa2, 'i', 'd', 0x07, 0xa4, 'j', 'u', 'n', 'k'}
		data = append(data, bytes.Repeat([]byte{0x91}, depth-2)...) // fixarray of one item
		return append(data, 0x90)                                   // empty fixarray
	}

	var got payload
	require.NoError(t, schema.Deserialize(nested(10000), &got))
	assert.Equal(t, payload{ID: 7}, got)

	for _, depth := range []int{10001, 10_000_000} {
		kept := payload{ID: 93}
		require.ErrorContains(t, schema.Deserialize(nested(depth), &kept), "nesting exceeds 10000 levels")
		assert.Equal(t, payload{ID: 93}, kept)
	}
}

type mapPayload struct {
	Scores map[string]int
	Nested map[int32]map[string]bool
	Inner  *payload
}

func (mapPayload) Name() string { return "map_payload" }

func TestSerializeEncodesMapsInStableOrder(t *testing.T) {
	t.Parallel()
	want := mapPayload{
		Scores: map[string]int{},
		Nested: map[int32]map[string]bool{},
		Inner:  &payload{ID: 1, Metadata: map[string]int{}},
	}
	for i := range 32 {
		key := strconv.Itoa(i)
		want.Scores[key] = i
		want.Nested[int32(i-16)] = map[string]bool{key: true, "x" + key: false}
		want.Inner.Metadata[key] = -i
	}

	first, err := schema.Serialize(want)
	require.NoError(t, err)
	for range 20 {
		again, err := schema.Serialize(want)
		require.NoError(t, err)
		require.Equal(t, first, again, "equal maps must encode to equal bytes")
	}

	var got mapPayload
	require.NoError(t, schema.Deserialize(first, &got))
	assert.Equal(t, want, got)
}

// inventory has only a helper method, so it still gets the sorted map encoder.
type inventory map[string]int

func (i inventory) Total() int {
	total := 0
	for _, n := range i {
		total += n
	}
	return total
}

// ownCodecMap encodes itself through a pointer receiver, which the sorted map encoder must not
// replace.
type ownCodecMap map[string]int

func (*ownCodecMap) EncodeMsgpack(e *msgpack.Encoder) error { return e.EncodeString("own codec") }

type namedMapPayload struct {
	Items inventory
	Own   ownCodecMap
}

func (namedMapPayload) Name() string { return "named_map_payload" }

func TestSerializeKeepsOwnMapCodec(t *testing.T) {
	t.Parallel()
	p := &namedMapPayload{Items: inventory{}, Own: ownCodecMap{"a": 1}}
	for i := range 32 {
		p.Items[strconv.Itoa(i)] = i
	}

	first, err := schema.Serialize(p) // through a pointer, so Own is addressable
	require.NoError(t, err)
	assert.Contains(t, string(first), "own codec")
	for range 20 {
		again, err := schema.Serialize(p)
		require.NoError(t, err)
		require.Equal(t, first, again, "a map type with only helper methods must encode in a stable order")
	}
}

type meters float32

type floatPayload struct {
	Plain  float32
	Named  meters
	Ptr    *float32
	List   []float32
	Arr    [2]meters
	ByKey  map[string]meters
	Frozen immutable.Slice[meters]
}

func (floatPayload) Name() string { return "float_payload" }

func TestDeserializeWidensFloat32(t *testing.T) {
	t.Parallel()
	schema.Register[floatPayload]()

	// 64-bit floats, as sent by JSON-fed tools and C# double fields.
	data, err := msgpack.Marshal(map[string]any{
		"Plain":  2.25,
		"Named":  2.5,
		"Ptr":    2.75,
		"List":   []float64{3.25},
		"Arr":    []float64{3.5, 3.75},
		"ByKey":  map[string]float64{"x": 4.25},
		"Frozen": []float64{4.5},
	})
	require.NoError(t, err)
	var got floatPayload
	require.NoError(t, schema.Deserialize(data, &got))
	ptr := float32(2.75)
	require.Equal(t, floatPayload{
		Plain:  2.25,
		Named:  2.5,
		Ptr:    &ptr,
		List:   []float32{3.25},
		Arr:    [2]meters{3.5, 3.75},
		ByKey:  map[string]meters{"x": 4.25},
		Frozen: immutable.SliceOf[meters](4.5),
	}, got)

	data, err = msgpack.Marshal(map[string]any{"Plain": 1e39})
	require.NoError(t, err)
	require.ErrorContains(t, schema.Deserialize(data, &got), "overflows float32")
}

func TestSerializeKeepsFloat32Width(t *testing.T) {
	t.Parallel()
	schema.Register[floatPayload]()
	data, err := schema.Serialize(floatPayload{Plain: 1.5, Named: 2.5})
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, msgpack.Unmarshal(data, &raw))
	assert.IsType(t, float32(0), raw["Plain"])
	assert.IsType(t, float32(0), raw["Named"])
}

// percent decodes itself and rejects negative values.
type percent float32

func (p *percent) DecodeMsgpack(d *msgpack.Decoder) error {
	f, err := d.DecodeFloat32()
	if err != nil {
		return err
	}
	if f < 0 {
		return errors.New("negative percent")
	}
	*p = percent(f)
	return nil
}

type percentPayload struct{ Share percent }

func (percentPayload) Name() string { return "percent_payload" }

func TestRegisterKeepsCustomFloat32Decoder(t *testing.T) {
	t.Parallel()
	schema.Register[percentPayload]()
	data, err := msgpack.Marshal(map[string]any{"Share": float32(-1)})
	require.NoError(t, err)
	var got percentPayload
	require.ErrorContains(t, schema.Deserialize(data, &got), "negative percent")
}

type unsupportedPayload struct{ Channel chan int }

func (unsupportedPayload) Name() string { return "unsupported" }

func TestSerializeReportsUnsupportedType(t *testing.T) {
	t.Parallel()
	_, err := schema.Serialize(unsupportedPayload{Channel: make(chan int)})
	require.ErrorContains(t, err, "failed to serialize")
}
