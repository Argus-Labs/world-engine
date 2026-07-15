package cardinal

import (
	"testing"

	"github.com/invopop/jsonschema"
	"github.com/shamaton/msgpack/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/known/structpb"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
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

type schemaSampleCodec struct{}

func (schemaSampleCodec) Marshal(Command) ([]byte, error) { return nil, nil }

func (schemaSampleCodec) Unmarshal([]byte) (Command, error) { return schemaSample{}, nil }

func (schemaSampleCodec) MessageDescriptor() protoreflect.MessageDescriptor {
	return (&cardinalv1.Snapshot{}).ProtoReflect().Descriptor()
}

// TestIntrospectSchemaNamesMatchWireFormat guards the introspect↔serialize
// contract for components/events (which are still msgpack): the field names the
// introspection schema advertises must equal the keys shamaton/msgpack actually
// reads and writes, so a client that fills one from the schema isn't silently
// dropped on the wire. Regression for the create-player "nickname" mismatch.
func TestIntrospectSchemaNamesMatchWireFormat(t *testing.T) {
	t.Parallel()

	// Names the wire format actually uses.
	encoded, err := msgpack.Marshal(schemaSample{Tagged: "a", JSONOnly: "b", Plain: 1, Skipped: "x"})
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, msgpack.Unmarshal(encoded, &wire))

	// Names introspection advertises, via the real register() path.
	d := &debugModule{
		components: make(map[string]*structpb.Struct),
		reflector: &jsonschema.Reflector{
			Anonymous:      true, // Don't add $id based on package path
			ExpandedStruct: true, // Inline the struct fields directly
			FieldNameTag:   "msgpack",
		},
	}
	require.NoError(t, d.register("component", schemaSample{}))
	schemaMap := d.components["schema-sample"].AsMap()
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

// TestCommandSchemaUsesProtoFieldNames guards the command contract, which is different: commands are
// proto wire, whose field names are the Go field names, so their advertised schema must use Go names
// (never the msgpack tag) to line up with the resolved proto message's field names on the client.
func TestCommandSchemaUsesProtoFieldNames(t *testing.T) {
	RegisterCommandCodec(schemaSample{}.Name(), schemaSampleCodec{})

	d := &debugModule{
		commands:          make(map[string]*structpb.Struct),
		commandProtoTypes: make(map[string]protoreflect.MessageDescriptor),
		commandReflector: &jsonschema.Reflector{
			Anonymous:      true,
			ExpandedStruct: true,
			FieldNameTag:   "protowire",
		},
	}
	require.NoError(t, d.register("command", schemaSample{}))
	assert.Equal(
		t,
		protoreflect.FullName("worldengine.cardinal.v1.Snapshot"),
		d.commandProtoTypes[schemaSample{}.Name()].FullName(),
		"registration must use the descriptor supplied by the codec, not reflect on the command type",
	)
	props, ok := d.commands["schema-sample"].AsMap()["properties"].(map[string]any)
	require.True(t, ok, "schema should have properties")

	assert.Contains(t, props, "Tagged")      // Go field name, NOT the msgpack "nickname"
	assert.Contains(t, props, "JSONOnly")    // json tag ignored
	assert.Contains(t, props, "Plain")       // untagged
	assert.Contains(t, props, "Skipped")     // exported => in the proto message => advertised
	assert.NotContains(t, props, "nickname") // the msgpack tag must not drive proto field names
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func systemNodeDescriptor() protoreflect.MessageDescriptor {
	return (&cardinalv1.SystemNode{}).ProtoReflect().Descriptor()
}

// TestCollectDescriptorSetFromRegistry checks collectDescriptorSet against SystemNode's real compiled
// fields, standing in for a generated command message.
func TestCollectDescriptorSetFromRegistry(t *testing.T) {
	md := systemNodeDescriptor()

	set := collectDescriptorSet([]protoreflect.MessageDescriptor{md})
	require.NotNil(t, set)

	dp := findMessageDescriptor(set, "SystemNode")
	require.NotNil(t, dp, "descriptor set must contain SystemNode's real message descriptor")
	require.Len(t, dp.GetField(), 2)
	assert.Equal(t, "id", dp.GetField()[0].GetName())
	assert.Equal(t, int32(1), dp.GetField()[0].GetNumber())
	assert.Equal(t, "name", dp.GetField()[1].GetName())
	assert.Equal(t, int32(2), dp.GetField()[1].GetNumber())

	// debug.proto imports google/protobuf/struct.proto transitively; the walk must pull it in too.
	assert.True(t, hasFile(set, "google/protobuf/struct.proto"), "transitive import must be included")

	assert.Nil(t, collectDescriptorSet(nil), "no resolved commands should yield no descriptor set")
}

// TestMarshalDescriptorSetRoundTrips checks the bytes Introspect ships decode back to the same
// descriptor, and that no resolved commands means nil (not an empty FileDescriptorSet's encoding,
// which happens to look the same, but nil is the honest value).
func TestMarshalDescriptorSetRoundTrips(t *testing.T) {
	md := systemNodeDescriptor()

	data, err := marshalDescriptorSet([]protoreflect.MessageDescriptor{md})
	require.NoError(t, err)
	require.NotEmpty(t, data)

	var decoded descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(data, &decoded))
	require.NotNil(t, findMessageDescriptor(&decoded, "SystemNode"))

	nilData, err := marshalDescriptorSet(nil)
	require.NoError(t, err)
	assert.Nil(t, nilData)
}

func findMessageDescriptor(set *descriptorpb.FileDescriptorSet, name string) *descriptorpb.DescriptorProto {
	for _, f := range set.GetFile() {
		for _, m := range f.GetMessageType() {
			if m.GetName() == name {
				return m
			}
		}
	}
	return nil
}

func hasFile(set *descriptorpb.FileDescriptorSet, path string) bool {
	for _, f := range set.GetFile() {
		if f.GetName() == path {
			return true
		}
	}
	return false
}

// TestCollectDescriptorSetDedupsSharedFiles guards against a duplicated FileDescriptorProto when two
// commands resolve to messages from the same file — duplicate paths make the descriptor catalog
// ambiguous for dynamic clients.
func TestCollectDescriptorSetDedupsSharedFiles(t *testing.T) {
	systemNode := systemNodeDescriptor()

	set := collectDescriptorSet([]protoreflect.MessageDescriptor{
		systemNode,
		systemNode,
	})

	seen := map[string]bool{}
	for _, f := range set.GetFile() {
		require.False(t, seen[f.GetName()], "duplicate file in descriptor set: %s", f.GetName())
		seen[f.GetName()] = true
	}
}

// TestCollectDescriptorSetIsDeterministic guards against Go's randomized map iteration leaking into
// the marshaled bytes: two commands resolving to messages in different files must produce the same
// FileDescriptorSet.File order every call, not just per-process. Uses SystemNode (debug.proto) and
// Snapshot (snapshot.proto) so there are two distinct files to potentially reorder.
func TestCollectDescriptorSetIsDeterministic(t *testing.T) {
	types := []protoreflect.MessageDescriptor{
		systemNodeDescriptor(),
		(&cardinalv1.Snapshot{}).ProtoReflect().Descriptor(),
	}

	want := collectDescriptorSet(types)
	var wantNames []string
	for _, f := range want.GetFile() {
		wantNames = append(wantNames, f.GetName())
	}

	for range 20 {
		got := collectDescriptorSet(types)
		var gotNames []string
		for _, f := range got.GetFile() {
			gotNames = append(gotNames, f.GetName())
		}
		require.Equal(t, wantNames, gotNames)
	}
}

func TestProtoDescriptorSetIsCached(t *testing.T) {
	d := &debugModule{commandProtoTypes: map[string]protoreflect.MessageDescriptor{
		"system-node": systemNodeDescriptor(),
	}}

	first, err := d.protoDescriptorSet()
	require.NoError(t, err)
	require.NotEmpty(t, first)

	d.commandProtoTypes["snapshot"] = (&cardinalv1.Snapshot{}).ProtoReflect().Descriptor()
	second, err := d.protoDescriptorSet()
	require.NoError(t, err)
	assert.Equal(t, first, second, "descriptor bytes should be built once after registration")
}
