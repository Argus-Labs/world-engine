package mcp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
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

// oneofField is scalarField plus a oneof index, so a oneof member descriptor is one line.
func oneofField(
	name string,
	number int32,
	typ descriptorpb.FieldDescriptorProto_Type,
	oneofIndex int32,
) *descriptorpb.FieldDescriptorProto {
	f := scalarField(name, number, typ)
	f.OneofIndex = new(oneofIndex)
	return f
}

// optionalField marks a scalar as proto3 optional. A proto3 optional field is encoded by protoc
// as a synthetic oneof: the field carries proto3_optional=true and a oneof_index, and the
// message declares a matching synthetic oneof named _<field>. The caller must add that oneof
// decl at the returned index via withProto3OptionalOneof.
func optionalField(
	name string,
	number int32,
	typ descriptorpb.FieldDescriptorProto_Type,
) *descriptorpb.FieldDescriptorProto {
	f := scalarField(name, number, typ)
	f.Proto3Optional = new(true)
	f.OneofIndex = new(int32(0))
	return f
}

// withProto3OptionalOneof returns the synthetic oneof decl an optionalField expects. It pairs
// with optionalField so a proto3 optional scalar is built exactly as protoc emits it.
func withProto3OptionalOneof(field string) *descriptorpb.OneofDescriptorProto {
	return &descriptorpb.OneofDescriptorProto{Name: new("_" + field)}
}

// dynamicNew builds a dynamic message for a descriptor and returns it; messageToMap is then
// driven directly so the presence rules are exercised without the get_state pipeline.
func dynamicNew(t *testing.T, md protoreflect.MessageDescriptor) protoreflect.Message {
	t.Helper()
	return dynamicpb.NewMessage(md)
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

// -------------------------------------------------------------------------------------------------
// messageToMap presence tests
//
// messageToMap is the environment a get_state where clause filters against. proto3 oneof
// members and proto3 optional scalars carry presence: an inactive/unset member must render as
// nil so the clause can tell "branch not chosen" from "branch chosen with the zero value." The
// guard was keyed on fd.Message() != nil, which only caught *message* members; scalar/enum
// oneof members (and optional scalars) fell through to msg.Get and rendered their zero value.
// These tests pin the fd.HasPresence()-based guard for every field category.
// -------------------------------------------------------------------------------------------------

// modeDescriptor builds `Mode { oneof which { string Tag = 1; int32 Team = 2; } }` — the
// minimal oneof-bearing component the bug report reproduces with.
func modeDescriptor(t *testing.T) protoreflect.MessageDescriptor {
	t.Helper()
	cmd := &descriptorpb.DescriptorProto{
		Name:      new("Mode"),
		OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: new("which")}},
		Field: []*descriptorpb.FieldDescriptorProto{
			oneofField("Tag", 1, descriptorpb.FieldDescriptorProto_TYPE_STRING, 0),
			oneofField("Team", 2, descriptorpb.FieldDescriptorProto_TYPE_INT32, 0),
		},
	}
	return testMessageDescriptor(t, testDescriptorSet(t, cmd), "Mode")
}

// An active scalar oneof member renders its value, while the inactive scalar oneof member
// renders nil (not 0/""), so a where clause can distinguish the two — the regression that
// drove the fix.
func TestMessageToMap_OneofScalarInactiveRendersNil(t *testing.T) {
	t.Parallel()

	md := modeDescriptor(t)
	tag := md.Fields().ByName("Tag")
	team := md.Fields().ByName("Team")

	// Active Team=0; Tag is an inactive scalar oneof member and must render nil, not "".
	msg := dynamicNew(t, md)
	msg.Set(team, protoreflect.ValueOfInt32(0))
	assert.Equal(t, map[string]any{"Tag": nil, "Team": int32(0)}, messageToMap(msg),
		"active Team=0 renders 0; inactive Tag renders nil, not the empty string")

	// Active Tag="red"; Team is an inactive scalar oneof member and must render nil, not 0.
	msg = dynamicNew(t, md)
	msg.Set(tag, protoreflect.ValueOfString("red"))
	assert.Equal(t, map[string]any{"Tag": "red", "Team": nil}, messageToMap(msg),
		"active Tag renders its value; inactive Team renders nil, not the int32 zero")

	// Fully unset oneof: both members render nil. Before the fix both rendered zero values,
	// making the "no branch chosen" state unrepresentable.
	msg = dynamicNew(t, md)
	assert.Equal(t, map[string]any{"Tag": nil, "Team": nil}, messageToMap(msg),
		"with no oneof branch chosen, both members render nil")
}

// An inactive enum oneof member renders nil, not the zero enum value's name. This is the
// scalar-oneof bug applied to enums, where the zero rendering was a first-enumerator name.
func TestMessageToMap_OneofEnumInactiveRendersNil(t *testing.T) {
	t.Parallel()

	cmd := &descriptorpb.DescriptorProto{
		Name:      new("Color"),
		OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: new("which")}},
		EnumType: []*descriptorpb.EnumDescriptorProto{{
			Name: new("Channel"),
			Value: []*descriptorpb.EnumValueDescriptorProto{
				{Name: new("RED"), Number: new(int32(0))},
				{Name: new("BLUE"), Number: new(int32(1))},
			},
		}},
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:       new("Hue"),
				Number:     new(int32(1)),
				Label:      descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:       descriptorpb.FieldDescriptorProto_TYPE_ENUM.Enum(),
				TypeName:   new(".test.Color.Channel"),
				OneofIndex: new(int32(0)),
			},
			oneofField("Tag", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, 0),
		},
	}
	md := testMessageDescriptor(t, testDescriptorSet(t, cmd), "Color")
	hue := md.Fields().ByName("Hue")

	// Hue active and set to RED(0) — the zero enumerator — renders its name "RED".
	msg := dynamicNew(t, md)
	msg.Set(hue, protoreflect.ValueOfEnum(0))
	assert.Equal(t, map[string]any{"Hue": "RED", "Tag": nil}, messageToMap(msg),
		"active enum oneof member set to the zero enumerator renders that name")

	// Hue inactive (Tag chosen instead) renders nil, not "RED".
	msg = dynamicNew(t, md)
	msg.Set(md.Fields().ByName("Tag"), protoreflect.ValueOfString("red"))
	assert.Equal(t, map[string]any{"Hue": nil, "Tag": "red"}, messageToMap(msg),
		"inactive enum oneof member renders nil, not the zero enumerator's name")
}

// An inactive *message* oneof member already rendered nil through the old fd.Message() guard.
// This test guards that the HasPresence() switch keeps that behavior.
func TestMessageToMap_OneofMessageInactiveRendersNil(t *testing.T) {
	t.Parallel()

	cmd := &descriptorpb.DescriptorProto{
		Name:      new("Wrapper"),
		OneofDecl: []*descriptorpb.OneofDescriptorProto{{Name: new("which")}},
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:       new("Inner"),
				Number:     new(int32(1)),
				Label:      descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:       descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName:   new(".test.Point"),
				OneofIndex: new(int32(0)),
			},
			oneofField("Tag", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, 0),
		},
	}
	raw := testDescriptorSet(t, cmd,
		&descriptorpb.DescriptorProto{Name: new("Point"), Field: []*descriptorpb.FieldDescriptorProto{
			scalarField("X", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32),
		}},
	)
	md := testMessageDescriptor(t, raw, "Wrapper")

	// Tag chosen; Inner is an inactive message oneof member and renders nil.
	msg := dynamicNew(t, md)
	msg.Set(md.Fields().ByName("Tag"), protoreflect.ValueOfString("red"))
	assert.Equal(t, map[string]any{"Inner": nil, "Tag": "red"}, messageToMap(msg),
		"inactive message oneof member still renders nil (regression guard for the old message guard)")

	// Inner chosen; recurses into the nested message map.
	msg = dynamicNew(t, md)
	inner := dynamicpb.NewMessage(testMessageDescriptor(t, raw, "Point"))
	inner.Set(inner.Descriptor().Fields().ByName("X"), protoreflect.ValueOfInt32(7))
	msg.Set(md.Fields().ByName("Inner"), protoreflect.ValueOfMessage(inner))
	assert.Equal(t, map[string]any{"Inner": map[string]any{"X": int32(7)}, "Tag": nil}, messageToMap(msg),
		"active message oneof member recurses; inactive scalar member renders nil")
}

// A proto3 optional scalar (synthetic oneof) that is unset renders nil, distinguishing "not set"
// from "explicitly zero." This is the same presence path the fix opens for oneof members.
func TestMessageToMap_Proto3OptionalUnsetRendersNil(t *testing.T) {
	t.Parallel()

	cmd := &descriptorpb.DescriptorProto{
		Name: new("Score"),
		OneofDecl: []*descriptorpb.OneofDescriptorProto{
			withProto3OptionalOneof("Value"),
		},
		Field: []*descriptorpb.FieldDescriptorProto{
			optionalField("Value", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32),
			scalarField("Plain", 2, descriptorpb.FieldDescriptorProto_TYPE_INT32),
		},
	}
	md := testMessageDescriptor(t, testDescriptorSet(t, cmd), "Score")
	value := md.Fields().ByName("Value")

	// Unset optional renders nil; plain scalar (no presence) renders its zero value.
	msg := dynamicNew(t, md)
	assert.Equal(t, map[string]any{"Value": nil, "Plain": int32(0)}, messageToMap(msg),
		"unset optional scalar renders nil; plain scalar renders zero")

	// Optional set to zero renders 0 — presence preserved, zero value honored.
	msg = dynamicNew(t, md)
	msg.Set(value, protoreflect.ValueOfInt32(0))
	assert.Equal(t, map[string]any{"Value": int32(0), "Plain": int32(0)}, messageToMap(msg),
		"optional scalar set to zero renders 0, distinguishable from unset")
}

// A plain unpresence-bearing scalar (no optional, no oneof) keeps rendering its zero value
// after the fix — the existing get_state contract that the mcp where-clause tests rely on.
func TestMessageToMap_PlainScalarUnsetRendersZero(t *testing.T) {
	t.Parallel()

	cmd := &descriptorpb.DescriptorProto{
		Name: new("Health"),
		Field: []*descriptorpb.FieldDescriptorProto{
			scalarField("HP", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64),
		},
	}
	md := testMessageDescriptor(t, testDescriptorSet(t, cmd), "Health")

	// No presence: unset renders the zero value, the contract every existing get_state fixture
	// (Health{HP}, Position{X}) depends on.
	assert.Equal(t, map[string]any{"HP": int64(0)}, messageToMap(dynamicNew(t, md)))
}

// List and map fields report no presence, so the fix never calls msg.Has on them (which would
// panic). An unset list renders an empty []any and an unset map an empty map — the same
// empty-collection render as before.
func TestMessageToMap_ListAndMapUnsetRenderEmptyAndDoNotPanic(t *testing.T) {
	t.Parallel()

	cmd := &descriptorpb.DescriptorProto{
		Name: new("Bag"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:   new("Items"),
				Number: new(int32(1)),
				Label:  descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				Type:   descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum(),
			},
			{
				Name:     new("Pairs"),
				Number:   new(int32(2)),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_REPEATED.Enum(),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: new(".test.Bag.PairsEntry"),
			},
		},
		// A map field's synthetic entry message is nested in the same scope as the field, which
		// is what makes protodesc recognize it as a map (not a plain repeated message).
		NestedType: []*descriptorpb.DescriptorProto{{
			Name:    new("PairsEntry"),
			Options: &descriptorpb.MessageOptions{MapEntry: new(true)},
			Field: []*descriptorpb.FieldDescriptorProto{
				{Name: new("key"), Number: new(int32(1)), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum()},
				{Name: new("value"), Number: new(int32(2)), Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_INT32.Enum()},
			},
		}},
	}
	raw := testDescriptorSet(t, cmd)
	md := testMessageDescriptor(t, raw, "Bag")

	got := messageToMap(dynamicNew(t, md))
	assert.Equal(t, []any{}, got["Items"], "unset repeated field renders an empty slice, not nil")
	assert.Equal(t, map[string]any{}, got["Pairs"], "unset map field renders an empty map, not nil")
}

// A self-referential singular message field that is absent renders nil and the recursion
// terminates — the property the original guard's comment cites as its reason for existence.
// HasPresence() preserves it.
func TestMessageToMap_SelfReferentialMessageAbsentRendersNil(t *testing.T) {
	t.Parallel()

	cmd := &descriptorpb.DescriptorProto{
		Name: new("Node"),
		Field: []*descriptorpb.FieldDescriptorProto{
			{
				Name:     new("Next"),
				Number:   new(int32(1)),
				Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
				Type:     descriptorpb.FieldDescriptorProto_TYPE_MESSAGE.Enum(),
				TypeName: new(".test.Node"),
			},
		},
	}
	md := testMessageDescriptor(t, testDescriptorSet(t, cmd), "Node")

	// Absent Next renders nil; messageToMap does not recurse into the empty message a Get
	// would hand back, so the self-referential type terminates.
	assert.Equal(t, map[string]any{"Next": nil}, messageToMap(dynamicNew(t, md)),
		"absent self-referential message renders nil and terminates")
}
