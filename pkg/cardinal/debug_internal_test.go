package cardinal

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/introspect"
	templatecommand "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/gen/pkg/template/multi-shard/shards/game/command"
	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

// schemaSample deliberately carries JSON tags that disagree with its generated protobuf mapping.
// Introspection must follow the generated mapping for every registered type kind.
type schemaSample struct {
	ArgusAuthID string `json:"argus_auth_id"`
	X           uint32 `json:"x"`
	Y           uint32 `json:"y"`
}

func (schemaSample) Name() string { return "schema-sample" }

func (sample schemaSample) ToProto() *templatecommand.MovePlayer {
	return &templatecommand.MovePlayer{ArgusAuthID: sample.ArgusAuthID, X: sample.X, Y: sample.Y}
}

func (sample schemaSample) FromProto(message *templatecommand.MovePlayer) schemaSample {
	if message == nil {
		return sample
	}
	sample.ArgusAuthID = message.GetArgusAuthID()
	sample.X = message.GetX()
	sample.Y = message.GetY()
	return sample
}

func (sample schemaSample) MarshalWire() ([]byte, error) {
	return proto.Marshal(sample.ToProto())
}

func (schemaSample) ProtoDescriptor() protoreflect.MessageDescriptor {
	return (&templatecommand.MovePlayer{}).ProtoReflect().Descriptor()
}

func (sample schemaSample) UnmarshalWire(data []byte) (any, error) {
	var message templatecommand.MovePlayer
	if err := proto.Unmarshal(data, &message); err != nil {
		return nil, err
	}
	return sample.FromProto(&message), nil
}

type wireOnlySample struct{}

func (wireOnlySample) Name() string                      { return "wire-only" }
func (wireOnlySample) MarshalWire() ([]byte, error)      { return nil, nil }
func (wireOnlySample) UnmarshalWire([]byte) (any, error) { return wireOnlySample{}, nil }

type namedWireOnlySample struct{ name string }

func (sample namedWireOnlySample) Name() string               { return sample.name }
func (namedWireOnlySample) MarshalWire() ([]byte, error)      { return nil, nil }
func (namedWireOnlySample) UnmarshalWire([]byte) (any, error) { return namedWireOnlySample{}, nil }
func (namedWireOnlySample) ProtoDescriptor() protoreflect.MessageDescriptor {
	return (&templatecommand.MovePlayer{}).ProtoReflect().Descriptor()
}

type unresolvedDescriptorSample struct {
	descriptor protoreflect.MessageDescriptor
}

func (unresolvedDescriptorSample) Name() string                 { return "unresolved-descriptor" }
func (unresolvedDescriptorSample) MarshalWire() ([]byte, error) { return nil, nil }
func (unresolvedDescriptorSample) UnmarshalWire([]byte) (any, error) {
	return unresolvedDescriptorSample{}, nil
}
func (sample unresolvedDescriptorSample) ProtoDescriptor() protoreflect.MessageDescriptor {
	return sample.descriptor
}

type nilDescriptorSample struct{ schemaSample }

func (nilDescriptorSample) ProtoDescriptor() protoreflect.MessageDescriptor { return nil }

type mismatchedDescriptorSample struct {
	Missing string
}

func (mismatchedDescriptorSample) Name() string                 { return "mismatched" }
func (mismatchedDescriptorSample) MarshalWire() ([]byte, error) { return nil, nil }
func (mismatchedDescriptorSample) UnmarshalWire([]byte) (any, error) {
	return mismatchedDescriptorSample{}, nil
}
func (mismatchedDescriptorSample) ProtoDescriptor() protoreflect.MessageDescriptor {
	return (&templatecommand.MovePlayer{}).ProtoReflect().Descriptor()
}

func newIntrospectionTestModule() *debugModule {
	return &debugModule{
		world:   &World{world: ecs.NewWorld()},
		catalog: introspect.NewCatalog(),
	}
}

func TestIntrospectAdvertisesSharedProtobufMetadata(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	for _, kind := range []introspect.Kind{introspect.Command, introspect.Component, introspect.Event} {
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
		assert.Equal(t, schemaSample{}.ProtoDescriptor().FullName(), protoreflect.FullName(schema.GetProtoMessageName()))

		properties, ok := schema.GetSchema().AsMap()["properties"].(map[string]any)
		require.True(t, ok, "schema should have properties")
		assert.ElementsMatch(t, []string{"ArgusAuthID", "X", "Y"}, mapKeys(properties))
		for property := range properties {
			assert.NotNil(t, schemaSample{}.ProtoDescriptor().Fields().ByName(protoreflect.Name(property)))
		}
	}

	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(response.Msg.GetProtoDescriptorSet(), &set))
	require.NotNil(t, findMessageDescriptor(&set, "MovePlayer"))
}

func TestFinalizeRequiresGeneratedProtobufDescriptor(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspect.Command, wireOnlySample{}))
	require.NoError(t, d.register(introspect.Component, nilDescriptorSample{}))
	err := d.finalizeCatalog()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to inspect wire-only")
	assert.Contains(t, err.Error(), "failed to inspect schema-sample")
	assert.Contains(t, err.Error(), "regenerate wire code")
	assert.Less(t, strings.Index(err.Error(), "schema-sample"), strings.Index(err.Error(), "wire-only"))
}

func TestFinalizeRejectsDescriptorWithUnresolvedImport(t *testing.T) {
	t.Parallel()

	// Build a descriptor whose imported file is deliberately absent.
	file, err := (protodesc.FileOptions{AllowUnresolvable: true}).New(&descriptorpb.FileDescriptorProto{
		Name:       proto.String("unresolved.proto"),
		Package:    proto.String("test"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"missing.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("Command")},
		},
	}, nil)
	require.NoError(t, err)

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspect.Command, unresolvedDescriptorSample{
		descriptor: file.Messages().Get(0),
	}))
	err = d.finalizeCatalog()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to inspect unresolved-descriptor")
	assert.Contains(t, err.Error(), "unresolved imports")
}

func TestFinalizeIntrospectionCatalogRejectsLaterRegistration(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.finalizeCatalog())
	err := d.register(introspect.Command, schemaSample{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog is finalized")
}

func TestIntrospectionCatalogSortsTypesByName(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspect.Command, namedWireOnlySample{name: "z-command"}))
	require.NoError(t, d.register(introspect.Command, namedWireOnlySample{name: "a-command"}))
	require.NoError(t, d.finalizeCatalog())

	types := d.catalog.Commands()
	require.Len(t, types, 2)
	assert.Equal(t, "a-command", types[0].GetName())
	assert.Equal(t, "z-command", types[1].GetName())
}

func TestDebugRegistrationIsNilSafe(t *testing.T) {
	t.Parallel()

	var d *debugModule
	require.NoError(t, d.register(introspect.Command, schemaSample{}))
}

func TestIntrospectionCatalogRejectsNilGeneratedDescriptor(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspect.Command, nilDescriptorSample{}))
	err := d.finalizeCatalog()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generated protobuf descriptor is nil")
}

func TestIntrospectionCatalogUsesDescriptorFields(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspect.Command, mismatchedDescriptorSample{}))
	require.NoError(t, d.finalizeCatalog())

	schemaMap := d.catalog.Commands()[0].GetSchema().AsMap()
	properties := schemaMap["properties"].(map[string]any)
	assert.ElementsMatch(t, []string{"ArgusAuthID", "X", "Y"}, mapKeys(properties))
	assert.NotContains(t, properties, "Missing")
}

func TestIntrospectRequiresFinalizedCatalog(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	_, err := d.Introspect(context.Background(), (*connect.Request[cardinalv1.IntrospectRequest])(nil))
	require.Error(t, err)
	assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))
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

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}
