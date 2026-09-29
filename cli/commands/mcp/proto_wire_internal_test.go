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
		payload string
		want    []byte
	}{
		{
			name:    "int64 scalar",
			field:   scalarField("Count", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
			payload: `{"Count": 5}`,
			want:    []byte{0x08, 0x05}, // field 1 varint = 5
		},
		{
			name:    "string",
			field:   scalarField("Nickname", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING),
			payload: `{"Nickname": "hi"}`,
			want:    []byte{0x0a, 0x02, 0x68, 0x69},
		},
		{
			name:    "field number is honored",
			field:   scalarField("Damage", 2, descriptorpb.FieldDescriptorProto_TYPE_UINT32),
			payload: `{"Damage": 7}`,
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
			payload: `{"Nums": [1, 2, 3]}`,
			want:    []byte{0x0a, 0x03, 0x01, 0x02, 0x03},
		},
		{
			name:    "empty payload",
			field:   scalarField("Count", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
			payload: `{}`,
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
	got, err := encodeCommandPayload(testMessageDescriptor(t, raw, "Cmd"), `{"Origin": {"X": 1, "Y": 2}}`)
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
	_, err := encodeCommandPayload(testMessageDescriptor(t, raw, "Cmd"), `{"Nope": 1}`)
	require.Error(t, err)
}

// TestEncodeCommandPayload_PreservesLargeIntegers pins the bug fix: feeding the raw JSON string to
// protojson preserves int64/uint64 values above 2^53 bit-for-bit. The previous map[string]any path
// coerced every number through float64, silently rounding values above 2^53 (e.g. 9007199254740993
// became 9007199254740992 on the wire with no error) and rejecting near-MaxInt64 values with an
// error naming the float64-rounded number the caller never sent.
func TestEncodeCommandPayload_PreservesLargeIntegers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		field *descriptorpb.FieldDescriptorProto
		json  string
		want  any // native Go type decodeMessage returns (int64 or uint64)
	}{
		{
			name:  "int64 just above 2^53 was silently rounded before the fix",
			field: scalarField("N", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
			json:  `{"N": 9007199254740993}`,
			want:  int64(9007199254740993),
		},
		{
			name:  "int64 max",
			field: scalarField("N", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
			json:  `{"N": 9223372036854775807}`,
			want:  int64(9223372036854775807),
		},
		{
			name:  "int64 min",
			field: scalarField("N", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
			json:  `{"N": -9223372036854775808}`,
			want:  int64(-9223372036854775808),
		},
		{
			name:  "uint64 max",
			field: scalarField("N", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT64),
			json:  `{"N": 18446744073709551615}`,
			want:  uint64(18446744073709551615),
		},
		{
			name:  "uint64 above 2^63 and below max",
			field: scalarField("N", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT64),
			json:  `{"N": 10000000000000000000}`,
			want:  uint64(10000000000000000000),
		},
		{
			name: "repeated int64 above 2^53 stays exact per element",
			field: &descriptorpb.FieldDescriptorProto{
				Name:   new("Nums"),
				Number: new(int32(1)),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_INT64.Enum(),
			},
			json: `{"Nums": [9007199254740993, 9223372036854775807]}`,
			want: []any{int64(9007199254740993), int64(9223372036854775807)},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			raw := testDescriptorSet(t, &descriptorpb.DescriptorProto{
				Name:  new("Cmd"),
				Field: []*descriptorpb.FieldDescriptorProto{tt.field},
			})
			md := testMessageDescriptor(t, raw, "Cmd")

			wire, err := encodeCommandPayload(md, tt.json)
			require.NoError(t, err)

			decoded, err := decodeMessage(md, wire)
			require.NoError(t, err)
			assert.Equal(t, tt.want, decoded[tt.field.GetName()])
		})
	}
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
