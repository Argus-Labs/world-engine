package micro

import (
	"context"

	"buf.build/go/protovalidate"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"
)

// commandGroup is the endpoint group inter-shard commands are served under and sent to. Both
// directions read it from here so a shard can never listen on one subject and be sent another.
const commandGroup = "command"

// pingEndpoint answers liveness checks from other shards.
const pingEndpoint = "ping"

// CommandHandler receives an inbound inter-shard command that has already passed transport
// validation: the payload parsed, the proto validated, the sender proven to be a shard, and the
// destination proven to be this shard.
//
// What happens next belongs to the runtime — enqueue the command for a later tick, run it now
// inside a transaction, whatever the runtime does. Returning an error fails the request back to the
// sending shard with codes.Internal.
type CommandHandler func(ctx context.Context, cmd *iscv1.Command) error

// ServeCommands registers this service's inter-shard endpoints: a liveness ping, plus one endpoint
// per entry in names, and routes every inbound command to handler.
//
// The validation applied before handler runs is the security boundary between shards, and it lives
// here so that every runtime serving inter-shard commands enforces the same rules:
//
//   - the payload must parse as a Command and satisfy its proto constraints;
//   - the sender's persona must be a shard address, so a player cannot reach these endpoints by
//     impersonating one;
//   - the command's destination address must be this service's own address.
//
// Nothing is sent as a side effect of handling. Outbound commands go out only when the caller calls
// Client.SendCommand, so a runtime that must not emit before a transaction commits stays in
// control of when that happens.
func (s *Service) ServeCommands(names []string, handler CommandHandler) error {
	if handler == nil {
		return eris.New("command handler must not be nil")
	}

	if err := s.AddEndpoint(pingEndpoint, func(_ context.Context, req *Request) *Response {
		return NewSuccessResponse(req, nil)
	}); err != nil {
		return eris.Wrap(err, "failed to register ping handler")
	}

	group := s.AddGroup(commandGroup)
	for _, name := range names {
		if err := group.AddEndpoint(name, s.commandEndpoint(handler)); err != nil {
			return eris.Wrapf(err, "failed to register %s command handler", name)
		}
	}

	return nil
}

// commandEndpoint builds the Handler that validates an inbound command and passes it to handler.
func (s *Service) commandEndpoint(handler CommandHandler) Handler {
	return func(ctx context.Context, req *Request) *Response {
		select {
		case <-ctx.Done():
			return NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), codes.Canceled)
		default:
		}

		cmd := &iscv1.Command{}
		if err := req.Payload.UnmarshalTo(cmd); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), codes.InvalidArgument)
		}

		if err := protovalidate.Validate(cmd); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to validate command"), codes.InvalidArgument)
		}

		// A shard-to-shard command carries a shard address as its persona. Anything else is a client
		// reaching for an endpoint it must not have.
		if _, err := ParseAddress(cmd.GetPersona().GetId()); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "command persona is not a shard address"), codes.InvalidArgument)
		}

		if String(s.Address) != String(cmd.GetAddress()) {
			return NewErrorResponse(req, eris.New("command address doesn't match shard address"), codes.InvalidArgument)
		}

		if err := handler(ctx, cmd); err != nil {
			return NewErrorResponse(req, eris.Wrap(err, "failed to handle command"), codes.Internal)
		}

		return NewSuccessResponse(req, nil)
	}
}

// SendCommand sends an inter-shard command to another shard and waits for its acknowledgement.
// persona identifies the sending shard and must be its address, since the receiver rejects a
// persona that is not one.
//
// The caller decides when this runs. A runtime that must not emit before its state is durable
// should buffer outbound commands and call this only after the commit succeeds — nothing here
// sends on its own.
func (c *Client) SendCommand(
	ctx context.Context,
	to *ServiceAddress,
	name string,
	persona string,
	payload []byte,
) error {
	cmd := &iscv1.Command{
		Name:    name,
		Address: to,
		Persona: &iscv1.Persona{Id: persona},
		Payload: payload,
	}

	if _, err := c.Request(ctx, to, commandGroup+"."+name, cmd); err != nil {
		return eris.Wrapf(err, "failed to send %s command", name)
	}

	return nil
}
