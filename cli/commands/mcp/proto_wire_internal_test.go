package mcp

import (
	"testing"

	"github.com/goccy/go-json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/argus-labs/world-engine/pkg/immutable"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

func testObjectSchema(field string, schema map[string]any) map[string]any {
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{field: schema},
	}
}

func TestEncodeCommandPayloadPreservesLargeIntegers(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		text string
		want any
	}{
		{"9007199254740993", int64(9007199254740993)},
		{"9223372036854775807", int64(9223372036854775807)},
		{"-9223372036854775808", int64(-9223372036854775808)},
		{"18446744073709551615", uint64(18446744073709551615)},
		{"10000000000000000000", uint64(10000000000000000000)},
	} {
		t.Run(tc.text, func(t *testing.T) {
			t.Parallel()
			blob, err := encodeCommandPayload(
				testObjectSchema("N", map[string]any{"type": "integer"}),
				`{"N":`+tc.text+`}`,
			)
			require.NoError(t, err)
			decoded, err := decodeMessage(blob)
			require.NoError(t, err)
			assert.Equal(t, tc.want, decoded["N"])
		})
	}
}

func TestEncodeCommandPayloadNestedArraysAndBytes(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"$ref": "#/$defs/Cmd",
		"$defs": map[string]any{
			"Cmd": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"cells": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "array", "items": map[string]any{"$ref": "#/$defs/Point"}},
					},
					"bytes": map[string]any{"type": "string", "contentEncoding": "base64"},
				},
			},
			"Point": testObjectSchema("X", map[string]any{"type": "integer"}),
		},
	}
	blob, err := encodeCommandPayload(schema, `{"cells":[[{"X":9007199254740993}],[{"X":2}]],"bytes":"AQID"}`)
	require.NoError(t, err)
	var target struct {
		Cells [][]struct{ X int64 } `msgpack:"cells"`
		Bytes []byte                `msgpack:"bytes"`
	}
	require.NoError(t, msgpack.Unmarshal(blob, &target))
	require.Len(t, target.Cells, 2)
	assert.Equal(t, int64(9007199254740993), target.Cells[0][0].X)
	assert.Equal(t, int64(2), target.Cells[1][0].X)
	assert.Equal(t, []byte{1, 2, 3}, target.Bytes)
	_, err = encodeCommandPayload(schema, `{"cells":[[{"nope":1}]]}`)
	require.Error(t, err)
}

func TestEncodeCommandPayloadRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	schema := testObjectSchema("N", map[string]any{"type": "integer", "minimum": -128, "maximum": 127})
	for _, payload := range []string{
		`{"Unknown":1}`, `{"N":"1"}`, `{"N":true}`, `{"N":1.5}`, `{"N":128}`, `{"N":-129}`,
		`null`, `[]`, `1`, `{`, `{} {}`, `{} trailing`,
	} {
		t.Run(payload, func(t *testing.T) {
			t.Parallel()
			_, err := encodeCommandPayload(schema, payload)
			require.Error(t, err)
		})
	}
	for _, number := range []string{"18446744073709551616", "-9223372036854775809"} {
		_, err := encodeCommandPayload(testObjectSchema("N", map[string]any{"type": "integer"}), `{"N":`+number+`}`)
		require.Error(t, err)
	}
	blob, err := encodeCommandPayload(schema, `{}`)
	require.NoError(t, err)
	decoded, err := decodeMessage(blob)
	require.NoError(t, err)
	assert.Empty(t, decoded)
}

func TestConvertTypeSchemasPreservesJSONSchema(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"$ref":  "#/$defs/Cmd",
		"$defs": map[string]any{"Cmd": testObjectSchema("N", map[string]any{"type": "integer"})},
	}
	metadata, err := structpb.NewStruct(schema)
	require.NoError(t, err)
	out := convertTypeSchemas([]*cardinalv1.TypeSchema{{Name: "cmd", JsonSchema: metadata}})
	require.Len(t, out, 1)
	assert.Equal(t, schema, out[0].Schema)
	// The map remains a JSON Schema rather than a rendered protobuf descriptor.
	raw, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"additionalProperties":false`)
}

func TestEncodeCommandPayloadNativeIntegerRanges(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ format, valid, invalid string }{
		{"int8", "127", "128"}, {"uint8", "255", "256"},
		{"int16", "32767", "32768"}, {"uint16", "65535", "65536"},
		{"int32", "2147483647", "2147483648"}, {"uint32", "4294967295", "4294967296"},
		{"int64", "9223372036854775807", "9223372036854775808"},
		{"uint64", "18446744073709551615", "-1"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			t.Parallel()
			schema := testObjectSchema("N", map[string]any{"type": "integer", "format": tc.format})
			_, err := encodeCommandPayload(schema, `{"N":`+tc.valid+`}`)
			require.NoError(t, err)
			_, err = encodeCommandPayload(schema, `{"N":`+tc.invalid+`}`)
			require.Error(t, err)
		})
	}
}

func TestCommandAndStateImmutableSlice(t *testing.T) {
	t.Parallel()
	schema := testObjectSchema(
		"Nums",
		map[string]any{"type": "array", "items": map[string]any{"type": "integer", "format": "int64"}},
	)
	blob, err := encodeCommandPayload(schema, `{"Nums":[9007199254740993,9223372036854775807]}`)
	require.NoError(t, err)
	var target struct{ Nums immutable.Slice[int64] }
	require.NoError(t, msgpack.Unmarshal(blob, &target))
	require.Equal(t, 2, target.Nums.Len())
	assert.Equal(t, int64(9007199254740993), target.Nums.At(0))
	assert.Equal(t, int64(9223372036854775807), target.Nums.At(1))
	state, err := msgpack.Marshal(target)
	require.NoError(t, err)
	components := entityComponents([]string{"Numbers"}, [][]byte{state}, componentDescriptors{"Numbers": schema})
	filter, err := compileStateFilter(GetStateInput{Where: "Numbers.Nums[0] == 9007199254740993"})
	require.NoError(t, err)
	matched, err := filter.matchesWhere(1, components)
	require.NoError(t, err)
	assert.True(t, matched)
}

func TestEncodeCommandPayloadScalarTypes(t *testing.T) {
	t.Parallel()
	schema := map[string]any{
		"type": "object", "additionalProperties": false,
		"properties": map[string]any{
			"Name":    map[string]any{"type": "string"},
			"Enabled": map[string]any{"type": "boolean"},
			"Ratio":   map[string]any{"anyOf": []any{map[string]any{"type": "number"}, map[string]any{"type": "null"}}},
		},
	}
	for _, payload := range []string{`{"Name":"hi","Enabled":true,"Ratio":1.5}`, `{"Ratio":null}`} {
		blob, err := encodeCommandPayload(schema, payload)
		require.NoError(t, err)
		var target struct {
			Name    string
			Enabled bool
			Ratio   *float64
		}
		require.NoError(t, msgpack.Unmarshal(blob, &target))
		if target.Ratio != nil {
			assert.Equal(t, 1.5, *target.Ratio)
			assert.Equal(t, "hi", target.Name)
			assert.True(t, target.Enabled)
		}
	}
}

func TestCommandAndStateIntegerMapKeys(t *testing.T) {
	t.Parallel()
	schema := testObjectSchema("Counts", map[string]any{
		"type": "object", "x-msgpack-key-format": "uint64",
		"additionalProperties": map[string]any{"type": "integer", "format": "int8"},
	})
	blob, err := encodeCommandPayload(schema, `{"Counts":{"18446744073709551615":-7}}`)
	require.NoError(t, err)
	var target struct{ Counts map[uint64]int8 }
	require.NoError(t, msgpack.Unmarshal(blob, &target))
	assert.Equal(t, map[uint64]int8{^uint64(0): -7}, target.Counts)
	state, err := msgpack.Marshal(target)
	require.NoError(t, err)
	decoded, err := decodeMessage(state)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"18446744073709551615": int8(-7)}, decoded["Counts"])
	for _, payload := range []string{`{"Counts":{"-1":0}}`, `{"Counts":{"1":128}}`, `{"Counts":{"1":0,"01":1}}`} {
		_, err = encodeCommandPayload(schema, payload)
		require.Error(t, err)
	}
}

func TestMalformedMessagePackStateStaysRaw(t *testing.T) {
	t.Parallel()
	// {"M": {[]: 1}} has an array as a map key, which cannot be a Go map key.
	blob := []byte{0x81, 0xa1, 'M', 0x81, 0x90, 0x01}
	got := entityComponents([]string{"Bad"}, [][]byte{blob}, componentDescriptors{"Bad": {}})
	assert.Equal(t, blob, got["Bad"])
}
