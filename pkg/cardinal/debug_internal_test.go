package cardinal

import (
	"context"
	"math"
	"reflect"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/ecs"
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

type scalarWireOnlySample string

func (scalarWireOnlySample) Name() string                      { return "scalar-wire-only" }
func (scalarWireOnlySample) MarshalWire() ([]byte, error)      { return nil, nil }
func (scalarWireOnlySample) UnmarshalWire([]byte) (any, error) { return scalarWireOnlySample(""), nil }

type namedWireOnlySample struct{ name string }

func (sample namedWireOnlySample) Name() string               { return sample.name }
func (namedWireOnlySample) MarshalWire() ([]byte, error)      { return nil, nil }
func (namedWireOnlySample) UnmarshalWire([]byte) (any, error) { return namedWireOnlySample{}, nil }

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

type nestedFormSample struct {
	Count uint32
}

type namedByte byte

type formSchemaSample struct {
	Tagged     string `json:"different_name"`
	Signed     int64
	Unsigned   uint64
	Small      int32
	Narrow     uint8
	NarrowList []int8
	Ratio      float32
	Optional   *string
	Fixed      [2]nestedFormSample
	Blob       []byte
	NamedBlob  []namedByte
	Created    time.Time
	Fallback   any
}

type jsonFallbackFormSample struct {
	Fixed   [1]int64
	Special [1]float32
	Data    *[]byte
}

func (formSchemaSample) Name() string                      { return "form-schema" }
func (formSchemaSample) MarshalWire() ([]byte, error)      { return nil, nil }
func (formSchemaSample) UnmarshalWire([]byte) (any, error) { return formSchemaSample{}, nil }

func newIntrospectionTestModule() *debugModule {
	return &debugModule{
		world:   &World{world: ecs.NewWorld()},
		catalog: newIntrospectionCatalog(),
	}
}

func TestIntrospectAdvertisesSharedProtobufMetadata(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	for _, kind := range []introspectionKind{introspectionCommand, introspectionComponent, introspectionEvent} {
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

func TestIntrospectAllowsTypesWithoutGeneratedProtobufDescriptor(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspectionCommand, wireOnlySample{}))
	require.NoError(t, d.finalizeCatalog())

	schemas := d.catalog.Commands()
	require.Len(t, schemas, 1)
	assert.Empty(t, schemas[0].GetProtoMessageName())
	assert.Empty(t, d.catalog.DescriptorSet())
}

func TestFinalizeAllowsScalarWireCodec(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspectionCommand, scalarWireOnlySample("")))
	require.NoError(t, d.finalizeCatalog())

	types := d.catalog.Commands()
	require.Len(t, types, 1)
	assert.Empty(t, types[0].GetSchema().AsMap())
}

func TestFinalizeOmitsDescriptorWithUnresolvedImport(t *testing.T) {
	t.Parallel()

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
	require.NoError(t, d.register(introspectionCommand, unresolvedDescriptorSample{
		descriptor: file.Messages().Get(0),
	}))
	require.NoError(t, d.finalizeCatalog())

	types := d.catalog.Commands()
	require.Len(t, types, 1)
	assert.Empty(t, types[0].GetProtoMessageName())
	assert.Empty(t, d.catalog.DescriptorSet())
}

func TestFinalizeIntrospectionCatalogRejectsLaterRegistration(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.finalizeCatalog())
	err := d.register(introspectionCommand, schemaSample{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog is finalized")
}

func TestIntrospectionCatalogSortsTypesByName(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspectionCommand, namedWireOnlySample{name: "z-command"}))
	require.NoError(t, d.register(introspectionCommand, namedWireOnlySample{name: "a-command"}))
	require.NoError(t, d.finalizeCatalog())

	types := d.catalog.Commands()
	require.Len(t, types, 2)
	assert.Equal(t, "a-command", types[0].GetName())
	assert.Equal(t, "z-command", types[1].GetName())
}

func TestDebugRegistrationIsNilSafe(t *testing.T) {
	t.Parallel()

	var d *debugModule
	require.NoError(t, d.register(introspectionCommand, schemaSample{}))
}

func TestIntrospectionCatalogRejectsNilGeneratedDescriptor(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspectionCommand, nilDescriptorSample{}))
	err := d.finalizeCatalog()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "generated protobuf descriptor is nil")
}

func TestIntrospectionCatalogRejectsDescriptorFieldMismatch(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspectionCommand, mismatchedDescriptorSample{}))
	err := d.finalizeCatalog()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "has no field for Go field Missing")
}

func TestFinalizeBuildsFormSchemaFromGoType(t *testing.T) {
	t.Parallel()

	d := newIntrospectionTestModule()
	require.NoError(t, d.register(introspectionCommand, formSchemaSample{}))
	require.NoError(t, d.finalizeCatalog())

	types := d.catalog.Commands()
	require.Len(t, types, 1)
	schemaMap := types[0].GetSchema().AsMap()
	properties := schemaMap["properties"].(map[string]any)
	assert.Contains(t, properties, "Tagged")
	assert.NotContains(t, properties, "different_name")

	required := schemaMap["required"].([]any)
	assert.NotContains(t, required, "Optional")
	assert.Contains(t, required, "Tagged")
	assert.Equal(t, "string", properties["Signed"].(map[string]any)["type"])
	assert.Equal(t, "^-?[0-9]+$", properties["Signed"].(map[string]any)["pattern"])
	assert.Equal(t, "string", properties["Unsigned"].(map[string]any)["type"])
	assert.Equal(t, "^[0-9]+$", properties["Unsigned"].(map[string]any)["pattern"])
	assert.Equal(t, "integer", properties["Small"].(map[string]any)["type"])
	assert.InDelta(t, 0, properties["Narrow"].(map[string]any)["minimum"], 0)
	assert.InDelta(t, 255, properties["Narrow"].(map[string]any)["maximum"], 0)
	narrowItems := properties["NarrowList"].(map[string]any)["items"].(map[string]any)
	assert.InDelta(t, -128, narrowItems["minimum"], 0)
	assert.InDelta(t, 127, narrowItems["maximum"], 0)
	assert.Len(t, properties["Ratio"].(map[string]any)["anyOf"], 2)

	fixed := properties["Fixed"].(map[string]any)
	assert.InDelta(t, 2, fixed["minItems"], 0)
	assert.InDelta(t, 2, fixed["maxItems"], 0)
	nestedKey := formDefinitionKey(reflect.TypeFor[nestedFormSample]())
	assert.Contains(t, schemaMap["$defs"], nestedKey)
	assert.Equal(t, "#/$defs/"+jsonPointerToken(nestedKey), fixed["items"].(map[string]any)["$ref"])
	assert.Equal(t, "base64", properties["Blob"].(map[string]any)["contentEncoding"])
	namedBlob := properties["NamedBlob"].(map[string]any)
	assert.Equal(t, "array", namedBlob["type"])
	assert.Equal(t, "integer", namedBlob["items"].(map[string]any)["type"])
	assert.Equal(t, "date-time", properties["Created"].(map[string]any)["format"])
	assert.Empty(t, properties["Fallback"].(map[string]any))
	assert.NotEmpty(t, schemaMap["$defs"])
}

func TestBuildFormSchemaUsesJSONValuesForFallbackBytes(t *testing.T) {
	t.Parallel()

	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    proto.String("fallback.proto"),
		Package: proto.String("test"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Fallback"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name: proto.String("Fixed"), Number: proto.Int32(1),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:  descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
				},
				{
					Name: proto.String("Special"), Number: proto.Int32(2),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:  descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
				},
				{
					Name: proto.String("Data"), Number: proto.Int32(3),
					Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:  descriptorpb.FieldDescriptorProto_TYPE_BYTES.Enum(),
				},
			},
		}},
	}, nil)
	require.NoError(t, err)

	schemaMap := buildFormSchema(jsonFallbackFormSample{}, file.Messages().Get(0))
	properties := schemaMap["properties"].(map[string]any)
	fixedItems := properties["Fixed"].(map[string]any)["items"].(map[string]any)
	assert.Equal(t, "integer", fixedItems["type"])
	assert.InDelta(t, 9007199254740991, fixedItems["maximum"], 0)
	specialItems := properties["Special"].(map[string]any)["items"].(map[string]any)
	assert.Equal(t, "number", specialItems["type"])
	assert.InDelta(t, math.MaxFloat32, specialItems["maximum"], 0)
	assert.NotContains(t, specialItems, "anyOf")
	data := properties["Data"].(map[string]any)
	assert.Equal(t, "string", data["type"])
	assert.NotContains(t, data, "contentEncoding")
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
	assert.True(t, hasFile(&set, "google/protobuf/struct.proto"), "transitive imports must be included")
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
