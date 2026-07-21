package command_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	_ "github.com/argus-labs/world-engine/pkg/cardinal/internal/commandtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type descriptorCodec struct{}

func (descriptorCodec) Marshal(command.Payload) ([]byte, error) { return []byte{}, nil }

func (descriptorCodec) Unmarshal([]byte) (command.Payload, error) { return testPayload{}, nil }

type testPayload struct{}

func (testPayload) Name() string { return "test-payload" }

func TestRegisterCodecRequiresMessageDescriptor(t *testing.T) {
	assert.PanicsWithError(
		t,
		"command \"test-missing-descriptor\" codec has no protobuf message descriptor "+
			"(regenerate it with world sdk generate)",
		func() { command.RegisterCodec("test-missing-descriptor", descriptorCodec{}, nil) },
	)
}

func TestMessageDescriptorReturnsRegisteredCodecDescriptor(t *testing.T) {
	descriptor := command.MessageDescriptor("simple_command")
	require.NotNil(t, descriptor)
	assert.Equal(t, protoreflect.FullName("cardinal.commandtest.SimpleCommand"), descriptor.FullName())
	assert.Nil(t, command.MessageDescriptor("test-unregistered-codec"))
}
