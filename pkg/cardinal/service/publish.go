package service

import (
	"context"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

func (s *ShardService) PublishDefaultEvent(evt event.Event) error {
	payload := evt.Payload.(event.EventPayload)

	// Craft target service address `<this cardinal's service address>.event.<group>.<event name>`.
	target := micro.String(s.Address) + ".event." + payload.Name()

	payloadPb, err := schema.ToProtoStruct(payload)
	if err != nil {
		return eris.Wrap(err, "failed to marshal event payload")
	}

	eventPb := &iscv1.Event{
		Name:    payload.Name(),
		Payload: payloadPb,
	}

	bytes, err := proto.Marshal(eventPb)
	if err != nil {
		return eris.Wrap(err, "failed to marshal iscv1.Event")
	}

	return s.client.Publish(target, bytes)
}

func (s *ShardService) PublishInterShardCommand(raw event.Event) error {
	isc, ok := raw.Payload.(command.Command)
	if !ok {
		return eris.Errorf("invalid inter shard command %v", isc)
	}

	payload, err := schema.ToProtoStruct(isc.Payload)
	if err != nil {
		return eris.Wrap(err, "failed to marshal command payload")
	}

	commandPb := &iscv1.Command{
		Name:    isc.Payload.Name(),
		Address: isc.Address,
		Persona: &iscv1.Persona{Id: isc.Persona},
		Payload: payload,
	}

	_, err = s.client.Request(context.Background(), isc.Address, "command."+isc.Payload.Name(), commandPb)
	if err != nil {
		return eris.Wrapf(err, "failed to send inter-shard command %s to shard", isc.Payload.Name())
	}

	return nil
}
