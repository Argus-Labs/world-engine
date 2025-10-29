// Package protoutil provides marshaling utilities for converting ECS types to protobuf types.
//
// This package exists to decouple the core ECS package from protobuf dependencies. Instead of
// having marshal methods directly on ECS types (like RawEvent.Marshal()), this package provides
// conversion functions that can be used by Cardinal and other higher-level packages that need to
// serialize ECS types to protobuf format.
package protoutil

import (
	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// MarshalCommand converts an ecs.Command to its protobuf representation.
func MarshalCommand(command ecs.Command, dst *micro.ServiceAddress, personaID string) (*iscv1.CommandBody, error) {
	pbStruct, err := marshalToStruct(command)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal command into structpb")
	}

	return &iscv1.CommandBody{
		Name:    command.Name(),
		Payload: pbStruct,
		Address: dst,
		Persona: &iscv1.Persona{
			Id: personaID,
		},
	}, nil
}

// MarshalEvent converts an ecs.Event to its protobuf representation.
func MarshalEvent(event ecs.Event) (*iscv1.Event, error) {
	pbStruct, err := marshalToStruct(event)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal event into structpb")
	}

	return &iscv1.Event{
		Name:    event.Name(),
		Payload: pbStruct,
	}, nil
}

func UnmarshalCommandBytes(bytes []byte) (*iscv1.CommandRaw, error) {
	command := &iscv1.CommandRaw{}
	if err := proto.Unmarshal(bytes, command); err != nil {
		return command, eris.Wrap(err, "failed to unmarshal command from command bytes")
	}
	if err := protovalidate.Validate(command); err != nil {
		return command, eris.Wrap(err, "failed to validate command")
	}
	return command, nil
}

// marshalToStruct is a helper function to convert an arbitrary struct type into a protobuf-
// compatible format.
func marshalToStruct(payload any) (*structpb.Struct, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal payload")
	}

	var m map[string]any
	if err := json.Unmarshal(bytes, &m); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal payload to map[string]any")
	}

	pbStruct, err := structpb.NewStruct(m)
	if err != nil {
		return nil, eris.Wrap(err, "failed to convert map to structpb.Struct")
	}
	return pbStruct, nil
}
