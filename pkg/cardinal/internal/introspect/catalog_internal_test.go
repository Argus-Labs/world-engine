package introspect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

func TestBuildFormSchemaFromProtobufDescriptor(t *testing.T) {
	t.Parallel()

	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    proto.String("form.proto"),
		Package: proto.String("test"),
		Syntax:  proto.String("proto3"),
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: proto.String("Mode"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: proto.String("MODE_UNSPECIFIED"), Number: proto.Int32(0)},
				{Name: proto.String("MODE_ACTIVE"), Number: proto.Int32(1)},
			},
		}},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("Child"),
				Field: []*descriptorpb.FieldDescriptorProto{
					formTestField("Count", 1, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
						descriptorpb.FieldDescriptorProto_TYPE_UINT32, ""),
				},
			},
			{
				Name: proto.String("Command"),
				NestedType: []*descriptorpb.DescriptorProto{{
					Name: proto.String("LabelsEntry"),
					Field: []*descriptorpb.FieldDescriptorProto{
						formTestField("key", 1, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
							descriptorpb.FieldDescriptorProto_TYPE_STRING, ""),
						formTestField("value", 2, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
							descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".test.Child"),
					},
					Options: &descriptorpb.MessageOptions{MapEntry: proto.Bool(true)},
				}},
				Field: []*descriptorpb.FieldDescriptorProto{
					formTestField("Signed", 1, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
						descriptorpb.FieldDescriptorProto_TYPE_INT64, ""),
					formTestField("Blob", 2, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
						descriptorpb.FieldDescriptorProto_TYPE_BYTES, ""),
					formTestField("Children", 3, descriptorpb.FieldDescriptorProto_LABEL_REPEATED,
						descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".test.Child"),
					formTestField("Labels", 4, descriptorpb.FieldDescriptorProto_LABEL_REPEATED,
						descriptorpb.FieldDescriptorProto_TYPE_MESSAGE, ".test.Command.LabelsEntry"),
					formTestField("Mode", 5, descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL,
						descriptorpb.FieldDescriptorProto_TYPE_ENUM, ".test.Mode"),
				},
			},
		},
	}, nil)
	require.NoError(t, err)

	schema := buildFormSchema(file.Messages().ByName("Command"))
	properties := schema["properties"].(map[string]any)
	assert.NotContains(t, schema, "required")
	assert.Equal(t, "^-?[0-9]+$", properties["Signed"].(map[string]any)["pattern"])
	assert.Equal(t, "base64", properties["Blob"].(map[string]any)["contentEncoding"])
	assert.Equal(t, "#/$defs/test.Child", properties["Children"].(map[string]any)["items"].(map[string]any)["$ref"])
	labels := properties["Labels"].(map[string]any)["additionalProperties"].(map[string]any)
	assert.Equal(t, "#/$defs/test.Child", labels["$ref"])
	assert.Equal(t, []any{"MODE_UNSPECIFIED", "MODE_ACTIVE"}, properties["Mode"].(map[string]any)["enum"])
	assert.Contains(t, schema["$defs"].(map[string]any), "test.Child")
}

func formTestField(
	name string,
	number int32,
	label descriptorpb.FieldDescriptorProto_Label,
	fieldType descriptorpb.FieldDescriptorProto_Type,
	typeName string,
) *descriptorpb.FieldDescriptorProto {
	field := &descriptorpb.FieldDescriptorProto{
		Name: proto.String(name), Number: proto.Int32(number), Label: label.Enum(), Type: fieldType.Enum(),
	}
	if typeName != "" {
		field.TypeName = proto.String(typeName)
	}
	return field
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

func hasFile(set *descriptorpb.FileDescriptorSet, path string) bool {
	for _, file := range set.GetFile() {
		if file.GetName() == path {
			return true
		}
	}
	return false
}
