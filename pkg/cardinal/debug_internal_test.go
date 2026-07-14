package cardinal

import (
	"testing"

	"github.com/invopop/jsonschema"
	"github.com/shamaton/msgpack/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
)

// schemaSample mixes the tag cases that decide a field's wire name: a msgpack
// tag, a json-only tag whose value differs from the field name, an untagged
// field, and an explicitly excluded field.
type schemaSample struct {
	Tagged   string `json:"tagged"   msgpack:"nickname"` // msgpack tag wins
	JSONOnly string `json:"jsonOnly"`                    // json tag ignored -> field name
	Plain    int    // no tags -> field name
	Skipped  string `msgpack:"-"` // excluded from the wire
}

func (schemaSample) Name() string { return "schema-sample" }

// TestIntrospectSchemaNamesMatchWireFormat guards the introspect↔serialize
// contract: the field names advertised by the introspection schema must equal
// the keys shamaton/msgpack actually reads and writes, so a client that fills a
// command/component from the schema isn't silently dropped on the wire.
// Regression for the create-player "nickname" mismatch.
func TestIntrospectSchemaNamesMatchWireFormat(t *testing.T) {
	t.Parallel()

	// Names the wire format actually uses.
	encoded, err := msgpack.Marshal(schemaSample{Tagged: "a", JSONOnly: "b", Plain: 1, Skipped: "x"})
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, msgpack.Unmarshal(encoded, &wire))

	// Names introspection advertises, via the real register() path.
	d := &debugModule{
		commands:  make(map[string]*structpb.Struct),
		reflector: &jsonschema.Reflector{
			Anonymous:      true, // Don't add $id based on package path
			ExpandedStruct: true, // Inline the struct fields directly
			FieldNameTag:   "msgpack",
		},
	}
	require.NoError(t, d.register("command", schemaSample{}))
	schemaMap := d.commands["schema-sample"].AsMap()
	props, ok := schemaMap["properties"].(map[string]any)
	require.True(t, ok, "schema should have properties")

	assert.ElementsMatch(t, mapKeys(wire), mapKeys(props),
		"introspect schema field names must match the msgpack wire keys")

	// Spot-check the specifics the fix turns on.
	assert.Contains(t, props, "nickname")   // msgpack tag wins over json
	assert.Contains(t, props, "JSONOnly")   // json tag ignored; Go field name used
	assert.Contains(t, props, "Plain")      // untagged -> field name
	assert.NotContains(t, props, "Skipped") // msgpack:"-" excluded
	assert.NotContains(t, props, "tagged")  // the json tag value must not leak through
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
