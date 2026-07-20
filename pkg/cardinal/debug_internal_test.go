package cardinal

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/shamaton/msgpack/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
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

func schemaSampleDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()

	field := func(
		name string,
		number int32,
		typeName descriptorpb.FieldDescriptorProto_Type,
	) *descriptorpb.FieldDescriptorProto {
		return &descriptorpb.FieldDescriptorProto{
			Name:     proto.String(name),
			JsonName: proto.String(name),
			Number:   proto.Int32(number),
			Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
			Type:     typeName.Enum(),
		}
	}
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    proto.String("cardinal/schema_sample.proto"),
		Package: proto.String("cardinal.test"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("SchemaSample"),
			Field: []*descriptorpb.FieldDescriptorProto{
				field("Tagged", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
				field("JSONOnly", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING),
				field("Plain", 3, descriptorpb.FieldDescriptorProto_TYPE_INT64),
				field("Skipped", 4, descriptorpb.FieldDescriptorProto_TYPE_STRING),
			},
		}},
	}, nil)
	require.NoError(t, err)
	return file.Messages().ByName("SchemaSample")
}

func newIntrospectionTestModule() *debugModule {
	world := &World{world: ecs.NewWorld(), options: WorldOptions{TickRate: 20}}
	debug := newDebugModule(world)
	world.debug = &debug
	return &debug
}

// TestIntrospectSchemaNamesMatchWireFormat guards the introspect↔serialize contract for
// components/events, which still use msgpack.
func TestIntrospectSchemaNamesMatchWireFormat(t *testing.T) {
	t.Parallel()

	encoded, err := msgpack.Marshal(schemaSample{Tagged: "a", JSONOnly: "b", Plain: 1, Skipped: "x"})
	require.NoError(t, err)
	var wire map[string]any
	require.NoError(t, msgpack.Unmarshal(encoded, &wire))

	d := newIntrospectionTestModule()
	require.NoError(t, d.register("component", schemaSample{}, nil))
	props, ok := d.components["schema-sample"].schema.AsMap()["properties"].(map[string]any)
	require.True(t, ok, "schema should have properties")

	assert.ElementsMatch(t, mapKeys(wire), mapKeys(props))
	assert.Contains(t, props, "nickname")
	assert.Contains(t, props, "JSONOnly")
	assert.Contains(t, props, "Plain")
	assert.NotContains(t, props, "Skipped")
	assert.NotContains(t, props, "tagged")
}

// TestIntrospectAdvertisesCommandWireMetadata exercises the observable response contract: command
// schemas use Go/protobuf field names, identify their message, and ship a loadable descriptor set.
func TestIntrospectAdvertisesCommandWireMetadata(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	descriptor := schemaSampleDescriptor(t)
	require.NoError(t, d.register("command", schemaSample{}, descriptor))

	response, err := d.Introspect(
		context.Background(),
		connect.NewRequest(&cardinalv1.IntrospectRequest{}),
	)
	require.NoError(t, err)
	require.Len(t, response.Msg.GetCommands(), 1)

	commandSchema := response.Msg.GetCommands()[0]
	assert.Equal(t, string(descriptor.FullName()), commandSchema.GetProtoMessageName())
	props, ok := commandSchema.GetSchema().AsMap()["properties"].(map[string]any)
	require.True(t, ok, "schema should have properties")
	descriptorFieldNames := make([]string, descriptor.Fields().Len())
	for i := range descriptor.Fields().Len() {
		descriptorFieldNames[i] = string(descriptor.Fields().Get(i).Name())
	}
	assert.ElementsMatch(t, descriptorFieldNames, mapKeys(props))

	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(response.Msg.GetProtoDescriptorSet(), &set))
	files, err := protodesc.NewFiles(&set)
	require.NoError(t, err)
	resolved, err := files.FindDescriptorByName(descriptor.FullName())
	require.NoError(t, err)
	assert.Equal(t, descriptor.FullName(), resolved.FullName())
}

func mapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for key := range m {
		out = append(out, key)
	}
	return out
}

func systemNodeDescriptor() protoreflect.MessageDescriptor {
	return (&cardinalv1.SystemNode{}).ProtoReflect().Descriptor()
}

func TestCollectDescriptorSetDedupsSharedFiles(t *testing.T) {
	descriptor := systemNodeDescriptor()
	set := collectDescriptorSet([]protoreflect.MessageDescriptor{descriptor, descriptor})

	seen := make(map[string]bool, len(set.GetFile()))
	for _, file := range set.GetFile() {
		require.False(t, seen[file.GetName()], "duplicate file in descriptor set: %s", file.GetName())
		seen[file.GetName()] = true
	}
}

func TestMarshalDescriptorSetIsInputOrderIndependent(t *testing.T) {
	systemNode := systemNodeDescriptor()
	snapshot := (&cardinalv1.Snapshot{}).ProtoReflect().Descriptor()

	forward, err := marshalDescriptorSet([]protoreflect.MessageDescriptor{systemNode, snapshot})
	require.NoError(t, err)
	reverse, err := marshalDescriptorSet([]protoreflect.MessageDescriptor{snapshot, systemNode})
	require.NoError(t, err)
	assert.Equal(t, forward, reverse)

	empty, err := marshalDescriptorSet(nil)
	require.NoError(t, err)
	assert.Nil(t, empty)
}
