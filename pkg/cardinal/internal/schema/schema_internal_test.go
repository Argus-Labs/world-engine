package schema_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/stretchr/testify/require"
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

type unsupportedPayload struct{ Channel chan int }

func (unsupportedPayload) Name() string { return "unsupported" }

func TestSerializeReportsUnsupportedType(t *testing.T) {
	t.Parallel()
	_, err := schema.Serialize(unsupportedPayload{Channel: make(chan int)})
	require.ErrorContains(t, err, "failed to serialize")
}
