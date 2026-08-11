package introspect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	cardinalv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1"
)

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
