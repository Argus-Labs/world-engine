// Package commandtest registers protobuf wire codecs for the shared testutils command fixtures.
package commandtest

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/commandtest/commandtestpb"
	"github.com/argus-labs/world-engine/pkg/testutils"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

//nolint:gochecknoinits // registers test-fixture codecs on package load, mirroring generated code
func init() {
	command.RegisterCodec(
		"simple_command",
		simpleCodec{},
		(&commandtestpb.SimpleCommand{}).ProtoReflect().Descriptor(),
	)
	command.RegisterCodec("command_a", aCodec{}, (&commandtestpb.CommandA{}).ProtoReflect().Descriptor())
	command.RegisterCodec("command_b", bCodec{}, (&commandtestpb.CommandB{}).ProtoReflect().Descriptor())
	command.RegisterCodec("command_c", cCodec{}, (&commandtestpb.CommandC{}).ProtoReflect().Descriptor())
}

type simpleCodec struct{}

func (simpleCodec) Marshal(payload command.Payload) ([]byte, error) {
	value, ok := payload.(testutils.SimpleCommand)
	if !ok {
		return nil, eris.Errorf("expected SimpleCommand, got %T", payload)
	}
	return proto.Marshal(&commandtestpb.SimpleCommand{Value: int64(value.Value)})
}

func (simpleCodec) Unmarshal(data []byte) (command.Payload, error) {
	var message commandtestpb.SimpleCommand
	if err := proto.Unmarshal(data, &message); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal SimpleCommand")
	}
	return testutils.SimpleCommand{Value: int(message.GetValue())}, nil
}

type aCodec struct{}

func (aCodec) Marshal(payload command.Payload) ([]byte, error) {
	value, ok := payload.(testutils.CommandA)
	if !ok {
		return nil, eris.Errorf("expected CommandA, got %T", payload)
	}
	return proto.Marshal(&commandtestpb.CommandA{X: value.X, Y: value.Y, Z: value.Z})
}

func (aCodec) Unmarshal(data []byte) (command.Payload, error) {
	var message commandtestpb.CommandA
	if err := proto.Unmarshal(data, &message); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal CommandA")
	}
	return testutils.CommandA{X: message.GetX(), Y: message.GetY(), Z: message.GetZ()}, nil
}

type bCodec struct{}

func (bCodec) Marshal(payload command.Payload) ([]byte, error) {
	value, ok := payload.(testutils.CommandB)
	if !ok {
		return nil, eris.Errorf("expected CommandB, got %T", payload)
	}
	return proto.Marshal(&commandtestpb.CommandB{
		Id:      value.ID,
		Label:   value.Label,
		Enabled: value.Enabled,
	})
}

func (bCodec) Unmarshal(data []byte) (command.Payload, error) {
	var message commandtestpb.CommandB
	if err := proto.Unmarshal(data, &message); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal CommandB")
	}
	return testutils.CommandB{
		ID:      message.GetId(),
		Label:   message.GetLabel(),
		Enabled: message.GetEnabled(),
	}, nil
}

type cCodec struct{}

func (cCodec) Marshal(payload command.Payload) ([]byte, error) {
	value, ok := payload.(testutils.CommandC)
	if !ok {
		return nil, eris.Errorf("expected CommandC, got %T", payload)
	}
	return proto.Marshal(&commandtestpb.CommandC{
		Values:  append([]int32(nil), value.Values[:]...),
		Counter: uint32(value.Counter),
	})
}

func (cCodec) Unmarshal(data []byte) (command.Payload, error) {
	var message commandtestpb.CommandC
	if err := proto.Unmarshal(data, &message); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal CommandC")
	}
	if len(message.GetValues()) != len(testutils.CommandC{}.Values) {
		return nil, eris.Errorf(
			"expected %d CommandC values, got %d",
			len(testutils.CommandC{}.Values),
			len(message.GetValues()),
		)
	}
	result := testutils.CommandC{
		Counter: uint16(message.GetCounter()), //nolint:gosec // uint16 wire field
	}
	copy(result.Values[:], message.GetValues())
	return result, nil
}
