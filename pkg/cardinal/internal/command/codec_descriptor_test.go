package command_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

type descriptorCodec struct {
	descriptor protoreflect.MessageDescriptor
}

func (c descriptorCodec) MessageDescriptor() protoreflect.MessageDescriptor { return c.descriptor }

func (descriptorCodec) Marshal(command.Payload) ([]byte, error) { return []byte{}, nil }

func (descriptorCodec) Unmarshal([]byte) (command.Payload, error) { return testPayload{}, nil }

type testPayload struct{}

func (testPayload) Name() string { return "test-payload" }

func TestRegisterCodecRequiresMessageDescriptor(t *testing.T) {
	assert.PanicsWithError(
		t,
		"command \"test-missing-descriptor\" codec has no protobuf message descriptor "+
			"(regenerate it with world sdk generate)",
		func() { command.RegisterCodec("test-missing-descriptor", descriptorCodec{}) },
	)
}

func TestMessageDescriptorReturnsRegisteredCodecDescriptor(t *testing.T) {
	want := (&anypb.Any{}).ProtoReflect().Descriptor()
	command.RegisterCodec("test-described-codec", descriptorCodec{descriptor: want})

	assert.Equal(t, want, command.MessageDescriptor("test-described-codec"))
	assert.Nil(t, command.MessageDescriptor("test-unregistered-codec"))
}
