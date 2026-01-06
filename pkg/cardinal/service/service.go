package service

import (
	"sync"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/sign"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/rotisserie/eris"
)

// ShardService extends micro.Service with Cardinal specific functionality.
type ShardService struct {
	*micro.Service

	client     *micro.Client        // NATS client
	world      *ecs.World           // Reference to the ECS world
	tel        *telemetry.Telemetry // Telemetry for logging and tracing
	signer     sign.Signer          // Inter-shard command signer
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

	// TODO: make seed configurable
	signer, err := sign.NewSigner(opts.PrivateKey, 42)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create signer from private key")
	}
	s.signer = signer

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
	err := s.AddEndpoint("query", s.handleQuery)
	if err != nil {
		return eris.Wrap(err, "failed to register query handler")
	}
	err = s.AddEndpoint("introspect", s.handleIntrospect)
	if err != nil {
		return eris.Wrap(err, "failed to register introspect handler")
	}
	return nil
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

// ShardServiceOptions contains the configuration for creating a new ShardService.
type ShardServiceOptions struct {
	Client     *micro.Client         // NATS client for inter-service communication
	Address    *micro.ServiceAddress // This Cardinal shard's service address
	World      *ecs.World            // Reference to the ECS world
	Telemetry  *telemetry.Telemetry  // Telemetry for logging and tracing
	PrivateKey string                // Private key for signing inter-shard commands
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
	if opts.PrivateKey == "" {
		return eris.New("private key cannot be an empty string")
	}
	return nil
}
