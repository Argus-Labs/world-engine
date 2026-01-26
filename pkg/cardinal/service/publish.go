package service

import (
	"context"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/protoutil"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// InterShardCommand is a wrapper around an ecs.Command that contains the target shard and the command to send.
type InterShardCommand struct {
	Target  *micro.ServiceAddress
	Command command.CommandPayload
}

func (isc InterShardCommand) Name() string {
	return isc.Command.Name()
}

// Publish publishes a list of raw events.
func (s *ShardService) Publish(events []event.Event) {
	for _, evt := range events {
		go func(raw event.Event) {
			var err error
			switch raw.Kind {
			case event.KindDefault:
				err = s.publishEvent(raw)
			case event.KindInterShardCommand:
				err = s.publishInterShardCommand(raw)
			default:
				err = eris.Errorf("unknown event kind %T", raw.Kind)
			}
			if err != nil {
				logger := s.tel.GetLogger("publish")
				logger.Error().Err(err).Msg("Failed to publish raw event")
				return
			}
		}(evt)
	}
}

func (s *ShardService) publishEvent(raw event.Event) error {
	payload := raw.Payload

	// Craft target service address `<this cardinal's service address>.event.<group>.<event name>`.
	target := micro.String(s.Address) + ".event." + payload.Name()

	pbEvent, err := protoutil.MarshalEvent(payload)
	if err != nil {
		return eris.Wrap(err, "failed to marshal event")
	}

	bytes, err := proto.Marshal(pbEvent)
	if err != nil {
		return eris.Wrap(err, "failed to marshal iscv1.Event")
	}

	return s.client.Publish(target, bytes)
}

func (s *ShardService) publishInterShardCommand(raw event.Event) error {
	isc, ok := raw.Payload.(InterShardCommand)
	if !ok {
		return eris.Errorf("invalid inter shard command %v", isc)
	}

	pbCommand, err := protoutil.MarshalCommand(isc.Command, isc.Target, micro.String(s.Address))
	if err != nil {
		return eris.Wrap(err, "failed to marshal command")
	}

	_, err = s.client.Request(context.Background(), isc.Target, "command."+isc.Command.Name(), pbCommand)
	if err != nil {
		err = eris.Wrapf(err, "failed to send inter-shard command %s to shard", isc.Command.Name())
		return err
	}

	return nil
}
