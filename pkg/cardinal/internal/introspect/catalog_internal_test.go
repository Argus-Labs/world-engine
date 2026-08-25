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

type descriptorSample struct {
	name       string
	descriptor protoreflect.MessageDescriptor
}

func (sample descriptorSample) Name() string               { return sample.name }
func (descriptorSample) MarshalWire() ([]byte, error)      { return nil, nil }
func (descriptorSample) UnmarshalWire([]byte) (any, error) { return descriptorSample{}, nil }
func (sample descriptorSample) ProtoDescriptor() protoreflect.MessageDescriptor {
	return sample.descriptor
}

type wireOnlySample struct{ name string }

func (sample wireOnlySample) Name() string               { return sample.name }
func (wireOnlySample) MarshalWire() ([]byte, error)      { return nil, nil }
func (wireOnlySample) UnmarshalWire([]byte) (any, error) { return wireOnlySample{}, nil }

func TestFinalizeRequiresGeneratedProtobufDescriptor(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	require.NoError(t, catalog.Register(Command, wireOnlySample{name: "wire-only"}))
	require.NoError(t, catalog.Register(Component, descriptorSample{name: "nil-descriptor"}))
	err := catalog.Finalize()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to inspect nil-descriptor")
	assert.Contains(t, err.Error(), "failed to inspect wire-only")
	assert.Contains(t, err.Error(), "regenerate wire code")
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

	catalog := NewCatalog()
	require.NoError(t, catalog.Register(Command, descriptorSample{
		name:       "unresolved-descriptor",
		descriptor: file.Messages().Get(0),
	}))
	err = catalog.Finalize()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to inspect unresolved-descriptor")
	assert.Contains(t, err.Error(), "unresolved imports")
}

func TestFinalizeRejectsLaterRegistration(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog()
	require.NoError(t, catalog.Finalize())
	err := catalog.Register(Command, descriptorSample{name: "late"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog is finalized")
}

func TestFinalizeSortsTypesByName(t *testing.T) {
	t.Parallel()

	descriptor := (&cardinalv1.TypeSchema{}).ProtoReflect().Descriptor()
	catalog := NewCatalog()
	require.NoError(t, catalog.Register(Command, descriptorSample{name: "z-command", descriptor: descriptor}))
	require.NoError(t, catalog.Register(Command, descriptorSample{name: "a-command", descriptor: descriptor}))
	require.NoError(t, catalog.Finalize())

	types := catalog.Commands()
	require.Len(t, types, 2)
	assert.Equal(t, "a-command", types[0].GetName())
	assert.Equal(t, "z-command", types[1].GetName())
}

func TestBuildDescriptorSetIsDeterministicAndDeduplicated(t *testing.T) {
	t.Parallel()

	snapshot := (&cardinalv1.Snapshot{}).ProtoReflect().Descriptor()
	archetype := (&cardinalv1.Archetype{}).ProtoReflect().Descriptor()

	first, err := buildDescriptorSet([]protoreflect.MessageDescriptor{archetype, snapshot, archetype})
	require.NoError(t, err)
	second, err := buildDescriptorSet([]protoreflect.MessageDescriptor{snapshot, archetype})
	require.NoError(t, err)
	assert.Equal(t, first, second)

	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(first, &set))
	seen := make(map[string]bool, len(set.GetFile()))
	for _, file := range set.GetFile() {
		require.False(t, seen[file.GetName()], "duplicate file in descriptor set: %s", file.GetName())
		seen[file.GetName()] = true
	}
	assert.True(t, hasFile(&set, "google/protobuf/timestamp.proto"), "transitive imports must be included")
}

func hasFile(set *descriptorpb.FileDescriptorSet, path string) bool {
	for _, file := range set.GetFile() {
		if file.GetName() == path {
			return true
		}
	}
	return false
}

// -------------------------------------------------------------------------------------------------
// Fixed-array shape metadata
// -------------------------------------------------------------------------------------------------

type arrayShapes struct {
	Flat   [8]int32    // one dimension: the flat field is already unambiguous
	Grid   [4][8]int32 // two: 32 elements could be 4x8 or 8x4
	Cube   [2][3][4]int32
	Scalar int32
	Slice  []int32
	hidden [2][2]int32 //nolint:unused // unexported fields are never serialized
}

func (arrayShapes) Name() string                      { return "array_shapes" }
func (arrayShapes) MarshalWire() ([]byte, error)      { return nil, nil }
func (arrayShapes) UnmarshalWire([]byte) (any, error) { return arrayShapes{}, nil }

func TestArrayFields_OnlyMultiDimensional(t *testing.T) {
	t.Parallel()

	got := arrayFields(arrayShapes{})
	require.Len(t, got, 2, "only the multi-dimensional exported arrays carry a shape")

	assert.Equal(t, "Grid", got[0].GetField())
	assert.Equal(t, []uint32{4, 8}, got[0].GetDims())
	assert.Equal(t, "Cube", got[1].GetField())
	assert.Equal(t, []uint32{2, 3, 4}, got[1].GetDims())
}

func TestArrayFields_IgnoresNonStructs(t *testing.T) {
	t.Parallel()

	assert.Nil(t, arrayFields(nil))
}
