package introspect

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/immutable"
)

type sample struct {
	Renamed  int    `msgpack:"wire" json:"other"`
	Skipped  int    `msgpack:"-"    json:"visible"`
	Default  string `               json:"not_default"`
	Grid     [2][3]int
	Nested   immutable.Slice[immutable.Slice[[2]int]]
	Objects  []map[string]*sample
	Bytes    [2]byte
	ByteList immutable.Slice[byte]
	Counts   map[uint64]int8
}

func (sample) Name() string { return "sample" }

type named string

func (n named) Name() string { return string(n) }

func TestCatalogLifecycle(t *testing.T) {
	t.Parallel()
	c := NewCatalog()
	for _, kind := range []Kind{Command, Component, Event} {
		require.NoError(t, c.Register(kind, named("z")))
		require.NoError(t, c.Register(kind, named("a")))
		require.NoError(t, c.Register(kind, named("a")))
	}
	require.NoError(t, c.Finalize())
	require.NoError(t, c.Finalize())
	require.ErrorContains(t, c.Register(Command, sample{}), "finalized")
	for _, types := range [][]string{
		{c.Commands()[0].GetName(), c.Commands()[1].GetName()},
		{c.Components()[0].GetName(), c.Components()[1].GetName()},
		{c.Events()[0].GetName(), c.Events()[1].GetName()},
	} {
		assert.Equal(t, []string{"a", "z"}, types)
	}
	assert.Equal(t, "string", c.Commands()[0].GetJsonSchema().AsMap()["type"])
}

func resolve(t *testing.T, root, node map[string]any) map[string]any {
	t.Helper()
	if ref, ok := node["$ref"].(string); ok {
		return root["$defs"].(map[string]any)[strings.TrimPrefix(ref, "#/$defs/")].(map[string]any)
	}
	return node
}

func TestCatalogMatchesMessagePack(t *testing.T) {
	t.Parallel()
	v := sample{Renamed: 7, Skipped: 99, Default: "text", Grid: [2][3]int{{1, 2, 3}, {4, 5, 6}},
		Nested: immutable.SliceOf(immutable.SliceOf([2]int{8, 9})),
		Bytes:  [2]byte{1, 2}, ByteList: immutable.SliceOf(byte(3), byte(4)),
		Objects: []map[string]*sample{{"child": {Renamed: 10}, "absent": nil}}}
	c := NewCatalog()
	require.NoError(t, c.Register(Command, v))
	require.NoError(t, c.Finalize())
	root := c.Commands()[0].GetJsonSchema().AsMap()
	properties := resolve(t, root, root)["properties"].(map[string]any)
	data, err := schema.Serialize(v)
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, msgpack.Unmarshal(data, &wire))
	assert.Len(t, properties, len(wire))
	for key := range wire {
		assert.Contains(t, properties, key)
	}
	assert.Contains(t, properties, "wire")
	assert.NotContains(t, properties, "other")
	assert.NotContains(t, properties, "Skipped")
	assert.Contains(t, properties, "Default")
	assert.Equal(t, false, resolve(t, root, root)["additionalProperties"])
	counts := properties["Counts"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)
	assert.Equal(t, "uint64", counts["x-msgpack-key-format"])
	assert.Equal(t, "int8", counts["additionalProperties"].(map[string]any)["format"])
	grid := properties["Grid"].(map[string]any)
	assert.InDelta(t, 2, grid["minItems"], 0)
	assert.InDelta(t, 2, grid["maxItems"], 0)
	row := grid["items"].(map[string]any)
	assert.InDelta(t, 3, row["minItems"], 0)
	assert.Len(t, wire["Grid"], 2)
	assert.Len(t, wire["Grid"].([]any)[0], 3)
	nested := properties["Nested"].(map[string]any)
	for range 2 {
		assert.Equal(t, "array", nested["type"])
		nested = nested["items"].(map[string]any)
	}
	assert.InDelta(t, 2, nested["maxItems"], 0)
	assert.Equal(t, []any{[]any{[]any{int8(8), int8(9)}}}, wire["Nested"])
	assert.Equal(t, []byte{1, 2}, wire["Bytes"])
	assert.Equal(t, "base64", properties["Bytes"].(map[string]any)["contentEncoding"])
	assert.Equal(t, []any{uint8(3), uint8(4)}, wire["ByteList"])
	assert.Equal(t, "array", properties["ByteList"].(map[string]any)["type"])
	objects := properties["Objects"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)
	assert.Equal(t, "array", objects["type"])
	objectMap := objects["items"].(map[string]any)["anyOf"].([]any)[0].(map[string]any)
	assert.Equal(t, "object", objectMap["type"])
	pointer := objectMap["additionalProperties"].(map[string]any)["anyOf"].([]any)
	assert.Equal(t, resolve(t, root, root), resolve(t, root, pointer[0].(map[string]any)))
	assert.Equal(t, "null", pointer[1].(map[string]any)["type"])
	children := wire["Objects"].([]any)[0].(map[string]any)
	assert.Nil(t, children["absent"])
	assert.Equal(t, int8(10), children["child"].(map[string]any)["wire"])
}

type Embedded struct {
	Value int `msgpack:"value" json:"wrong"`
}

type embeddedRenamed struct {
	Embedded `msgpack:"ignored_name"`
}

func (embeddedRenamed) Name() string { return "embedded" }

type embeddedNoInline struct {
	Embedded `msgpack:"nested,noinline"`
}

func (embeddedNoInline) Name() string { return "noinline" }

type embeddedConflict struct {
	Value int `msgpack:"value"`
	//nolint:embeddedstructfieldcheck // Field order exercises codec conflict handling.
	Embedded
}

func (embeddedConflict) Name() string { return "conflict" }

func TestCatalogEmbeddingMatchesMessagePack(t *testing.T) {
	t.Parallel()
	for _, value := range []schema.Serializable{
		embeddedRenamed{Embedded{Value: 1}},
		embeddedNoInline{Embedded{Value: 2}},
		embeddedConflict{Value: 3, Embedded: Embedded{Value: 4}},
	} {
		t.Run(value.Name(), func(t *testing.T) {
			t.Parallel()
			c := NewCatalog()
			require.NoError(t, c.Register(Command, value))
			require.NoError(t, c.Finalize())
			root := c.Commands()[0].GetJsonSchema().AsMap()
			properties := resolve(t, root, root)["properties"].(map[string]any)
			data, err := schema.Serialize(value)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, msgpack.Unmarshal(data, &wire))
			assert.Len(t, properties, len(wire))
			for key := range wire {
				assert.Contains(t, properties, key)
			}
		})
	}
}
