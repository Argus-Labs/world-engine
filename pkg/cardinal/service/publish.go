package service

import (
	"context"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/protoutil"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// Cardinal's custom event kinds.
const (
	// EventKindInterShardCommand is the event kind for inter-shard commands.
	EventKindInterShardCommand ecs.EventKind = iota + ecs.CustomEventKindStart
)

// InterShardCommand is a wrapper around an ecs.Command that contains the target shard and the command to send.
type InterShardCommand struct {
	Target  *micro.ServiceAddress
	Command ecs.Command
}

// Publish publishes a list of raw events.
func (s *ShardService) Publish(events []ecs.RawEvent) {
	for _, event := range events {
		go func(raw ecs.RawEvent) {
			var err error
			switch raw.Kind {
			case ecs.EventKindDefault:
				err = s.publishEvent(raw)
			case EventKindInterShardCommand:
				err = s.publishInterShardCommand(raw)
			default:
				err = eris.Errorf("unknown event kind %T", raw.Kind)
			}
			if err != nil {
				logger := s.tel.GetLogger("publish")
				logger.Error().Err(err).Msg("Failed to publish raw event")
				return
			}
		}(event)
	}
}

func (s *ShardService) publishEvent(raw ecs.RawEvent) error {
	event, ok := raw.Payload.(ecs.Event)
	if !ok {
		return eris.Errorf("invalid event %v", event)
	}

	// Craft target service address `<this cardinal's service address>.event.<group>.<event name>`.
	target := micro.String(s.Address) + ".event." + event.Name()

	pbEvent, err := protoutil.MarshalEvent(event)
	if err != nil {
		return eris.Wrap(err, "failed to marshal event")
	}

	payload, err := proto.Marshal(pbEvent)
	if err != nil {
		return eris.Wrap(err, "failed to marshal iscv1.Event")
	}

	return s.NATS().Publish(target, payload)
}

func (s *ShardService) publishInterShardCommand(raw ecs.RawEvent) error {
	isc, ok := raw.Payload.(InterShardCommand)
	if !ok {
		return eris.Errorf("invalid inter shard command %v", isc)
	}

	pbCommand, err := protoutil.MarshalCommand(isc.Command, isc.Target, s.personaID)
	if err != nil {
		return eris.Wrap(err, "failed to marshal command")
	}

	signedCommand, err := s.signer.SignCommand(pbCommand, iscv1.AuthInfo_AUTH_MODE_PERSONA)
	if err != nil {
		return eris.Wrap(err, "failed to sign inter-shard command")
	}

	_, err = s.client.Request(context.Background(), isc.Target, "command."+isc.Command.Name(), signedCommand)
	if err != nil {
		err = eris.Wrapf(err, "failed to send inter-shard command %s to shard", isc.Command.Name())
		return err
	}

	return nil
}
