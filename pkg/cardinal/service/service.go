package service

import (
	"context"
	"sync"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/rotisserie/eris"
)

// ShardService extends micro.Service with Cardinal specific functionality.
type ShardService struct {
	*micro.Service

	client     *micro.Client        // NATS client
	world      *ecs.World           // Reference to the ECS world
	tel        *telemetry.Telemetry // Telemetry for logging and tracing
	queryPool  sync.Pool            // Pool for query objects
	introspect Introspect           // Introspection metadata cache
}

// NewShardService creates a new shard service.
func NewShardService(opts ShardServiceOptions) (*ShardService, error) {
	if err := opts.Validate(); err != nil {
		return nil, eris.Wrap(err, "invalid options passed")
	}

	s := &ShardService{
		world:  opts.World,
		client: opts.Client,
		tel:    opts.Telemetry,
		queryPool: sync.Pool{
			New: func() any {
				return &Query{
					// Pre-allocate space for 8 components which should cover most cases.
					Find: make([]string, 0, 8),
				}
			},
		},
	}

	service, err := micro.NewService(opts.Client, opts.Address, opts.Telemetry)
	if err != nil {
		return s, eris.Wrap(err, "failed to create micro service")
	}

	s.Service = service

	if err := s.registerEndpoints(); err != nil {
		return s, eris.Wrap(err, "failed to register endpoints")
	}

	return s, nil
}

// RegisterEndpoints registers the service endpoints for handling requests.
func (s *ShardService) registerEndpoints() error {
	err := s.AddEndpoint("ping", s.handlePing)
	if err != nil {
		return eris.Wrap(err, "failed to register ping handler")
	}
	err = s.AddEndpoint("query", s.handleQuery)
	if err != nil {
		return eris.Wrap(err, "failed to register query handler")
	}
	err = s.AddEndpoint("introspect", s.handleIntrospect)
	if err != nil {
		return eris.Wrap(err, "failed to register introspect handler")
	}
	return nil
}

// handlePing responds to health-check requests. Used by NATS CLI or K8s probes to verify
// the shard is connected to NATS and running. Accepts empty or valid Request payload.
func (s *ShardService) handlePing(ctx context.Context, req *micro.Request) *micro.Response {
	return micro.NewSuccessResponse(req, nil)
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

// ShardServiceOptions contains the configuration for creating a new ShardService.
type ShardServiceOptions struct {
	Client    *micro.Client         // NATS client for inter-service communication
	Address   *micro.ServiceAddress // This Cardinal shard's service address
	World     *ecs.World            // Reference to the ECS world
	Telemetry *telemetry.Telemetry  // Telemetry for logging and tracing
}

// Validate checks that all required fields in ShardServiceOptions are not nil.
func (opts ShardServiceOptions) Validate() error {
	if opts.Client == nil {
		return eris.New("client cannot be nil")
	}
	if opts.Address == nil {
		return eris.New("address cannot be nil")
	}
	if opts.World == nil {
		return eris.New("world cannot be nil")
	}
	if opts.Telemetry == nil {
		return eris.New("telemetry cannot be nil")
	}
	return nil
}
