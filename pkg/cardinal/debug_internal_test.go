package cardinal

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

// schemaSample deliberately carries JSON and legacy msgpack tags that disagree with its generated
// protobuf mapping. Introspection must follow the generated mapping for every registered type kind.
type schemaSample struct {
	ID    uint32 `json:"id"    msgpack:"legacy_id"`
	Label string `json:"label" msgpack:"-"`
}

func (schemaSample) Name() string { return "schema-sample" }

func (sample schemaSample) ToProto() *cardinalv1.SystemNode {
	return &cardinalv1.SystemNode{Id: sample.ID, Name: sample.Label}
}

func (sample schemaSample) FromProto(message *cardinalv1.SystemNode) schemaSample {
	if message == nil {
		return sample
	}
	sample.ID = message.GetId()
	sample.Label = message.GetName()
	return sample
}

func (sample schemaSample) MarshalWire() ([]byte, error) {
	return proto.Marshal(sample.ToProto())
}

func (sample schemaSample) UnmarshalWire(data []byte) (any, error) {
	var message cardinalv1.SystemNode
	if err := proto.Unmarshal(data, &message); err != nil {
		return nil, err
	}
	return sample.FromProto(&message), nil
}

type wireOnlySample struct{}

func (wireOnlySample) Name() string                      { return "wire-only" }
func (wireOnlySample) MarshalWire() ([]byte, error)      { return nil, nil }
func (wireOnlySample) UnmarshalWire([]byte) (any, error) { return wireOnlySample{}, nil }

func newIntrospectionTestModule() *debugModule {
	return &debugModule{
		world:      &World{world: ecs.NewWorld()},
		commands:   make(map[string]introspectedType),
		events:     make(map[string]introspectedType),
		components: make(map[string]introspectedType),
		reflector: &jsonschema.Reflector{
			Anonymous:      true,
			ExpandedStruct: true,
			FieldNameTag:   "protowire",
		},
	}
}

func TestIntrospectAdvertisesSharedProtobufMetadata(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	for _, kind := range []string{"command", "component", "event"} {
		require.NoError(t, d.register(kind, schemaSample{}))
	}
	require.NoError(t, d.finalizeCatalog())

	response, err := d.Introspect(context.Background(), (*connect.Request[cardinalv1.IntrospectRequest])(nil))
	require.NoError(t, err)

	for _, schemas := range [][]*cardinalv1.TypeSchema{
		response.Msg.GetCommands(),
		response.Msg.GetComponents(),
		response.Msg.GetEvents(),
	} {
		require.Len(t, schemas, 1)
		schema := schemas[0]
		assert.Equal(t, "worldengine.cardinal.v1.SystemNode", schema.GetProtoMessageName())

		properties, ok := schema.GetSchema().AsMap()["properties"].(map[string]any)
		require.True(t, ok, "schema should have properties")
		assert.ElementsMatch(t, []string{"ID", "Label"}, mapKeys(properties))
		assert.NotContains(t, properties, "legacy_id")
		assert.NotContains(t, properties, "id")
		assert.NotContains(t, properties, "label")
	}

	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(response.Msg.GetProtoDescriptorSet(), &set))
	require.NotNil(t, findMessageDescriptor(&set, "SystemNode"))
	assert.True(t, hasFile(&set, "google/protobuf/struct.proto"), "transitive imports must be included")
}

func TestIntrospectAllowsTypesWithoutGeneratedProtobufDescriptor(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register("command", wireOnlySample{}))
	require.NoError(t, d.finalizeCatalog())

	schemas := d.buildTypeSchemas(d.commands)
	require.Len(t, schemas, 1)
	assert.Empty(t, schemas[0].GetProtoMessageName())
	assert.Empty(t, d.descriptorSet)
}

func TestFinalizeIntrospectionCatalogRejectsLaterRegistration(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.finalizeCatalog())
	err := d.register("command", schemaSample{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog is finalized")
}

func TestIntrospectRequiresFinalizedCatalog(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	_, err := d.Introspect(context.Background(), (*connect.Request[cardinalv1.IntrospectRequest])(nil))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
}

func TestBuildDescriptorSetIsDeterministicAndDeduplicated(t *testing.T) {
	t.Parallel()

	systemNode := (&cardinalv1.SystemNode{}).ProtoReflect().Descriptor()
	snapshot := (&cardinalv1.Snapshot{}).ProtoReflect().Descriptor()

	first, err := buildDescriptorSet([]protoreflect.MessageDescriptor{systemNode, snapshot, systemNode})
	require.NoError(t, err)
	second, err := buildDescriptorSet([]protoreflect.MessageDescriptor{snapshot, systemNode})
	require.NoError(t, err)
	assert.Equal(t, first, second)

	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(first, &set))
	seen := make(map[string]bool, len(set.GetFile()))
	for _, file := range set.GetFile() {
		require.False(t, seen[file.GetName()], "duplicate file in descriptor set: %s", file.GetName())
		seen[file.GetName()] = true
	}
}

func findMessageDescriptor(set *descriptorpb.FileDescriptorSet, name string) *descriptorpb.DescriptorProto {
	for _, file := range set.GetFile() {
		for _, message := range file.GetMessageType() {
			if message.GetName() == name {
				return message
			}
		}
	}
	return nil
}

func hasFile(set *descriptorpb.FileDescriptorSet, path string) bool {
	for _, file := range set.GetFile() {
		if file.GetName() == path {
			return true
		}
	}
	return false
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
