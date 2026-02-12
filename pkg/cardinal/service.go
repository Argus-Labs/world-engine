package cardinal

import (
	"context"
	"sync"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/schema"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
)

// service extends micro.Service with Cardinal specific functionality.
type service struct {
	world     *World              // Reference to the Cardinal world
	client    *micro.Client       // NATS Client
	service   *micro.Service      // NATS handler
	commands  map[string]struct{} // Set of commands to handle
	queryPool sync.Pool           // Pool for query objects
}

// newService creates a new shard service.
func newService(world *World) *service {
	s := &service{
		world:    world,
		client:   nil, // Will be initialized at Init
		service:  nil, // Will be initialized at Init
		commands: make(map[string]struct{}),
		queryPool: sync.Pool{
			New: func() any {
				return &query{
					// Pre-allocate space for 8 components which should cover most cases.
					find: make([]string, 0, 8),
				}
			},
		},
	}

	return s
}

func (s *service) init() error {
	client, err := micro.NewClient(micro.WithLogger(s.world.tel.GetLogger("service")))
	if err != nil {
		return eris.Wrap(err, "failed to initialize micro client")
	}
	s.client = client

	microService, err := micro.NewService(client, s.world.address, &s.world.tel)
	if err != nil {
		return eris.Wrap(err, "failed to create micro service")
	}
	s.service = microService

	// Register endpoints.
	if err = s.service.AddEndpoint("ping", s.handlePing); err != nil {
		return eris.Wrap(err, "failed to register ping handler")
	}

	if err = s.service.AddEndpoint("query", s.handleQuery); err != nil {
		return eris.Wrap(err, "failed to register query handler")
	}

	for cmd := range s.commands {
		if err := s.service.AddGroup("command").AddEndpoint(cmd, s.handleCommand); err != nil {
			return eris.Wrapf(err, "failed to register %s command handler", cmd)
		}
	}

	return nil
}

func (s *service) shutdown() error {
	if s.service != nil {
		if err := s.service.Close(); err != nil {
			return eris.Wrap(err, "failed to close micro.Service")
		}
	}

	s.client.Close()

	return nil
}

func (s *service) registerCommandHandler(name string) {
	s.commands[name] = struct{}{}
}

// -------------------------------------------------------------------------------------------------
// Request handlers
// -------------------------------------------------------------------------------------------------

// handlePing responds to health-check requests. Used by NATS CLI or K8s probes to verify
// the shard is connected to NATS and running. Accepts empty or valid Request payload.
func (s *service) handlePing(_ context.Context, req *micro.Request) *micro.Response {
	return micro.NewSuccessResponse(req, nil)
}

// handleQuery is in query.go.

// handleCommand receives commands from clients and enqueues it in the world's command manager.
func (s *service) handleCommand(ctx context.Context, req *micro.Request) *micro.Response {
	// Check if shard is shutting down.
	select {
	case <-ctx.Done():
		return micro.NewErrorResponse(req, eris.Wrap(ctx.Err(), "context cancelled"), codes.Canceled)
	default:
		// Continue processing.
	}

	cmd := &iscv1.Command{}
	if err := req.Payload.UnmarshalTo(cmd); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to parse request payload"), codes.InvalidArgument)
	}

	if err := protovalidate.Validate(cmd); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to validate command"), codes.InvalidArgument)
	}

	if micro.String(s.world.address) != micro.String(cmd.GetAddress()) {
		return micro.NewErrorResponse(req, eris.New("command address doesn't match shard address"), codes.InvalidArgument)
	}

	if err := s.world.commands.Enqueue(cmd); err != nil {
		return micro.NewErrorResponse(req, eris.Wrap(err, "failed to enqueue command"), codes.InvalidArgument)
	}

	return micro.NewSuccessResponse(req, nil)
}

// -------------------------------------------------------------------------------------------------
// Event publishers
// -------------------------------------------------------------------------------------------------

func (s *service) publishDefaultEvent(evt event.Event) error {
	payload, ok := evt.Payload.(event.Payload)
	if !ok {
		return eris.Errorf("invalid event payload type: %T", evt.Payload)
	}

	// Craft target service address `<this cardinal's service address>.event.<group>.<event name>`.
	target := micro.String(s.world.address) + ".event." + payload.Name()

	payloadPb, err := schema.Serialize(payload)
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

func (s *service) publishInterShardCommand(evt event.Event) error {
	isc, ok := evt.Payload.(command.Command)
	if !ok {
		return eris.Errorf("invalid inter shard command %v", isc)
	}

	payload, err := schema.Serialize(isc.Payload)
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
