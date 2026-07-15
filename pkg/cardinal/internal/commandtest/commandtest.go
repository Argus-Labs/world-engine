// Package commandtest registers protobuf wire codecs for the shared testutils command fixtures.
package commandtest

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

//nolint:gochecknoinits // registers test-fixture codecs on package load, mirroring generated code
func init() {
	descriptors := testMessageDescriptors()
	command.RegisterCodec("simple_command", simpleCodec{descriptors.ByName("SimpleCommand")})
	command.RegisterCodec("command_a", aCodec{descriptors.ByName("CommandA")})
	command.RegisterCodec("command_b", bCodec{descriptors.ByName("CommandB")})
	command.RegisterCodec("command_c", cCodec{descriptors.ByName("CommandC")})
}

func testMessageDescriptors() protoreflect.MessageDescriptors {
	file, err := protodesc.NewFile(&descriptorpb.FileDescriptorProto{
		Name:    proto.String("cardinal/commandtest.proto"),
		Package: proto.String("cardinal.commandtest"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{
			{Name: proto.String("SimpleCommand"), Field: []*descriptorpb.FieldDescriptorProto{
				field("Value", 1, descriptorpb.FieldDescriptorProto_TYPE_INT64, false),
			}},
			{Name: proto.String("CommandA"), Field: []*descriptorpb.FieldDescriptorProto{
				field("X", 1, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, false),
				field("Y", 2, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, false),
				field("Z", 3, descriptorpb.FieldDescriptorProto_TYPE_DOUBLE, false),
			}},
			{Name: proto.String("CommandB"), Field: []*descriptorpb.FieldDescriptorProto{
				field("ID", 1, descriptorpb.FieldDescriptorProto_TYPE_UINT64, false),
				field("Label", 2, descriptorpb.FieldDescriptorProto_TYPE_STRING, false),
				field("Enabled", 3, descriptorpb.FieldDescriptorProto_TYPE_BOOL, false),
			}},
			{Name: proto.String("CommandC"), Field: []*descriptorpb.FieldDescriptorProto{
				field("Values", 1, descriptorpb.FieldDescriptorProto_TYPE_INT32, true),
				field("Counter", 2, descriptorpb.FieldDescriptorProto_TYPE_UINT32, false),
			}},
		},
	}, nil)
	if err != nil {
		panic(eris.Wrap(err, "failed to build command test descriptors"))
	}
	return file.Messages()
}

func field(
	name string,
	number int32,
	typ descriptorpb.FieldDescriptorProto_Type,
	repeated bool,
) *descriptorpb.FieldDescriptorProto {
	label := descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL
	if repeated {
		label = descriptorpb.FieldDescriptorProto_LABEL_REPEATED
	}
	return &descriptorpb.FieldDescriptorProto{
		Name:     proto.String(name),
		JsonName: proto.String(name),
		Number:   proto.Int32(number),
		Label:    label.Enum(),
		Type:     typ.Enum(),
	}
}

func unmarshalMessage(data []byte, descriptor protoreflect.MessageDescriptor) (*dynamicpb.Message, error) {
	message := dynamicpb.NewMessage(descriptor)
	if err := proto.Unmarshal(data, message); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal test command")
	}
	return message, nil
}

type simpleCodec struct {
	descriptor protoreflect.MessageDescriptor
}

func (c simpleCodec) MessageDescriptor() protoreflect.MessageDescriptor { return c.descriptor }

func (c simpleCodec) Marshal(payload command.Payload) ([]byte, error) {
	value, ok := payload.(testutils.SimpleCommand)
	if !ok {
		return nil, eris.Errorf("expected SimpleCommand, got %T", payload)
	}
	message := dynamicpb.NewMessage(c.descriptor)
	message.Set(c.descriptor.Fields().ByName("Value"), protoreflect.ValueOfInt64(int64(value.Value)))
	return proto.Marshal(message)
}

func (c simpleCodec) Unmarshal(data []byte) (command.Payload, error) {
	message, err := unmarshalMessage(data, c.descriptor)
	if err != nil {
		return nil, err
	}
	return testutils.SimpleCommand{Value: int(message.Get(c.descriptor.Fields().ByName("Value")).Int())}, nil
}

type aCodec struct {
	descriptor protoreflect.MessageDescriptor
}

func (c aCodec) MessageDescriptor() protoreflect.MessageDescriptor { return c.descriptor }

func (c aCodec) Marshal(payload command.Payload) ([]byte, error) {
	value, ok := payload.(testutils.CommandA)
	if !ok {
		return nil, eris.Errorf("expected CommandA, got %T", payload)
	}
	message := dynamicpb.NewMessage(c.descriptor)
	message.Set(c.descriptor.Fields().ByName("X"), protoreflect.ValueOfFloat64(value.X))
	message.Set(c.descriptor.Fields().ByName("Y"), protoreflect.ValueOfFloat64(value.Y))
	message.Set(c.descriptor.Fields().ByName("Z"), protoreflect.ValueOfFloat64(value.Z))
	return proto.Marshal(message)
}

func (c aCodec) Unmarshal(data []byte) (command.Payload, error) {
	message, err := unmarshalMessage(data, c.descriptor)
	if err != nil {
		return nil, err
	}
	return testutils.CommandA{
		X: message.Get(c.descriptor.Fields().ByName("X")).Float(),
		Y: message.Get(c.descriptor.Fields().ByName("Y")).Float(),
		Z: message.Get(c.descriptor.Fields().ByName("Z")).Float(),
	}, nil
}

type bCodec struct {
	descriptor protoreflect.MessageDescriptor
}

func (c bCodec) MessageDescriptor() protoreflect.MessageDescriptor { return c.descriptor }

func (c bCodec) Marshal(payload command.Payload) ([]byte, error) {
	value, ok := payload.(testutils.CommandB)
	if !ok {
		return nil, eris.Errorf("expected CommandB, got %T", payload)
	}
	message := dynamicpb.NewMessage(c.descriptor)
	message.Set(c.descriptor.Fields().ByName("ID"), protoreflect.ValueOfUint64(value.ID))
	message.Set(c.descriptor.Fields().ByName("Label"), protoreflect.ValueOfString(value.Label))
	message.Set(c.descriptor.Fields().ByName("Enabled"), protoreflect.ValueOfBool(value.Enabled))
	return proto.Marshal(message)
}

func (c bCodec) Unmarshal(data []byte) (command.Payload, error) {
	message, err := unmarshalMessage(data, c.descriptor)
	if err != nil {
		return nil, err
	}
	return testutils.CommandB{
		ID:      message.Get(c.descriptor.Fields().ByName("ID")).Uint(),
		Label:   message.Get(c.descriptor.Fields().ByName("Label")).String(),
		Enabled: message.Get(c.descriptor.Fields().ByName("Enabled")).Bool(),
	}, nil
}

type cCodec struct {
	descriptor protoreflect.MessageDescriptor
}

func (c cCodec) MessageDescriptor() protoreflect.MessageDescriptor { return c.descriptor }

func (c cCodec) Marshal(payload command.Payload) ([]byte, error) {
	value, ok := payload.(testutils.CommandC)
	if !ok {
		return nil, eris.Errorf("expected CommandC, got %T", payload)
	}
	message := dynamicpb.NewMessage(c.descriptor)
	values := message.Mutable(c.descriptor.Fields().ByName("Values")).List()
	for _, item := range value.Values {
		values.Append(protoreflect.ValueOfInt32(item))
	}
	message.Set(c.descriptor.Fields().ByName("Counter"), protoreflect.ValueOfUint32(uint32(value.Counter)))
	return proto.Marshal(message)
}

func (c cCodec) Unmarshal(data []byte) (command.Payload, error) {
	message, err := unmarshalMessage(data, c.descriptor)
	if err != nil {
		return nil, err
	}
	values := message.Get(c.descriptor.Fields().ByName("Values")).List()
	if values.Len() != len(testutils.CommandC{}.Values) {
		return nil, eris.Errorf("expected %d CommandC values, got %d", len(testutils.CommandC{}.Values), values.Len())
	}
	result := testutils.CommandC{
		Counter: uint16(message.Get(c.descriptor.Fields().ByName("Counter")).Uint()), //nolint:gosec // uint16 wire field
	}
	for i := range values.Len() {
		result.Values[i] = int32(values.Get(i).Int()) //nolint:gosec // descriptor guarantees int32
	}
	return result, nil
}
