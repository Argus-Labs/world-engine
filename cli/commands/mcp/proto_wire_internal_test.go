package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// testDescriptorSet marshals a one-file FileDescriptorSet holding the given messages, mirroring
// what a shard's introspect response carries in proto_descriptor_set.
func testDescriptorSet(t *testing.T, messages ...*descriptorpb.DescriptorProto) []byte {
	t.Helper()
	raw, err := proto.Marshal(&descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{{
			Name:        new("test.proto"),
			Syntax:      new("proto3"),
			Package:     new("test"),
			MessageType: messages,
		}},
	})
	require.NoError(t, err)
	return raw
}

// testMessageDescriptor resolves test.<name> through the same path the tools use.
func testMessageDescriptor(t *testing.T, raw []byte, name string) protoreflect.MessageDescriptor {
	t.Helper()
	files, err := resolveDescriptorFiles(raw)
	require.NoError(t, err)
	md, err := findMessageDescriptor(files, "test."+name)
	require.NoError(t, err)
	return md
}

func scalarField(
	name string,
	number int32,
	typ descriptorpb.FieldDescriptorProto_Type,
) *descriptorpb.FieldDescriptorProto {
	return &descriptorpb.FieldDescriptorProto{
		Name:   new(name),
		Number: new(number),
		Label:  descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:   typ.Enum(),
	}
}

func TestEncodeCommandPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		field   *descriptorpb.FieldDescriptorProto
		payload map[string]any
		want    []byte
	}{
		{
			name:    "int64 scalar",
			field:   scalarField("Count", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
			payload: map[string]any{"Count": float64(5)},
			want:    []byte{0x08, 0x05}, // field 1 varint = 5
		},
		{
			name:    "string",
			field:   scalarField("Nickname", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
			payload: map[string]any{"Nickname": "hi"},
			want:    []byte{0x0a, 0x02, 0x68, 0x69},
		},
		{
			name:    "field number is honored",
			field:   scalarField("Damage", 2, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
			payload: map[string]any{"Damage": float64(7)},
			want:    []byte{0x10, 0x07},
		},
		{
			name: "packed repeated scalars",
			field: &descriptorpb.FieldDescriptorProto{
				Name:   new("Nums"),
				Number: new(int32(1)),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
			},
			payload: map[string]any{"Nums": []any{float64(1), float64(2), float64(3)}},
			want:    []byte{0x0a, 0x03, 0x01, 0x02, 0x03},
		},
		{
			name:    "empty payload",
			field:   scalarField("Count", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
			payload: map[string]any{},
			want:    []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw := testDescriptorSet(t, &descriptorpb.DescriptorProto{
				Name:  new("Cmd"),
				Field: []*descriptorpb.FieldDescriptorProto{tt.field},
			})
			got, err := encodeCommandPayload(testMessageDescriptor(t, raw, "Cmd"), tt.payload)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEncodeCommandPayloadNested(t *testing.T) {
	t.Parallel()

	origin := &descriptorpb.FieldDescriptorProto{
		Name:     new("Origin"),
		Number:   new(int32(1)),
		Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
		Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
		TypeName: new(".test.Point"),
	}
	raw := testDescriptorSet(t,
		&descriptorpb.DescriptorProto{Name: new("Cmd"), Field: []*descriptorpb.FieldDescriptorProto{origin}},
		&descriptorpb.DescriptorProto{Name: new("Point"), Field: []*descriptorpb.FieldDescriptorProto{
			scalarField("X", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32),
			scalarField("Y", 2, descriptorpb.FieldDescriptorProto_TYPE_INT32),
		}},
	)

	// Origin{X:1, Y:2}: field 1 (0x0a) len 4 -> [08 01 10 02]
	got, err := encodeCommandPayload(testMessageDescriptor(t, raw, "Cmd"), map[string]any{
		"Origin": map[string]any{"X": float64(1), "Y": float64(2)},
	})
	require.NoError(t, err)
	assert.Equal(t, []byte{0x0a, 0x04, 0x08, 0x01, 0x10, 0x02}, got)
}

func TestEncodeCommandPayloadRejectsUnknownField(t *testing.T) {
	t.Parallel()

	raw := testDescriptorSet(t, &descriptorpb.DescriptorProto{
		Name: new("Cmd"),
		Field: []*descriptorpb.FieldDescriptorProto{
			scalarField("Count", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
		},
	})
	_, err := encodeCommandPayload(testMessageDescriptor(t, raw, "Cmd"), map[string]any{"Nope": float64(1)})
	require.Error(t, err)
}

func TestResolveDescriptorFiles_EmptySetIsAnError(t *testing.T) {
	t.Parallel()

	_, err := resolveDescriptorFiles(nil)
	require.ErrorContains(t, err, "world sdk generate")
}

func TestResolveDescriptorFiles_GarbageIsAnError(t *testing.T) {
	t.Parallel()

	_, err := resolveDescriptorFiles([]byte{0xff, 0xff, 0xff})
	require.Error(t, err)
}

func TestFindMessageDescriptor_UnknownName(t *testing.T) {
	t.Parallel()

	raw := testDescriptorSet(t, &descriptorpb.DescriptorProto{Name: new("Cmd")})
	files, err := resolveDescriptorFiles(raw)
	require.NoError(t, err)

	_, err = findMessageDescriptor(files, "test.Missing")
	require.ErrorContains(t, err, "not found in shard descriptor set")

	_, err = findMessageDescriptor(files, "")
	require.ErrorContains(t, err, "no proto_message_name")
}
