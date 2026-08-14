package cardinal

import (
	"context"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/auth"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/shard"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// interShardSendTimeout bounds a single shard-to-shard send. See publishInterShardCommand for why
// this blocking is worth revisiting.
const interShardSendTimeout = 10 * time.Second

// service wires a world to the network: the shared client-facing server on one side, and NATS
// inter-shard commands on the other. The client-facing protocol itself lives in pkg/shard, so this
// world and any other runtime serving the same protocol cannot drift apart.
type service struct {
	world        *World
	shardServer  *shard.Server
	server       *http.Server
	log          zerolog.Logger
	authMode     AuthMode
	argusAuthURL string
	client       *micro.Client
	microService *micro.Service
	commands     map[string]struct{}
}

var _ shard.CommandSink = (*service)(nil)

// newService creates a new direct client-facing Cardinal service.
func newService(world *World, authMode AuthMode, argusAuthURL string) (*service, error) {
	s := &service{
		world:        world,
		log:          world.tel.GetLogger("service"),
		authMode:     authMode,
		argusAuthURL: argusAuthURL,
		commands:     make(map[string]struct{}),
	}

	shardServer, err := shard.NewServer(shard.Config{
		Address:  world.address,
		Commands: s,
		Logger:   s.log,
	})
	if err != nil {
		return nil, eris.Wrap(err, "failed to create shard server")
	}
	s.shardServer = shardServer

	return s, nil
}

// Submit implements shard.CommandSink and micro's inter-shard command handler: both the
// client-facing server and the ISC transport have already validated the command and proven it
// belongs to this shard, so the world's only remaining job is to queue it for the next tick.
func (s *service) Submit(_ context.Context, cmd *iscv1.Command) error {
	return s.world.commands.Enqueue(cmd)
}

// h2cProtocols enables HTTP/1.1 and unencrypted HTTP/2 (h2c), matching the
// previous h2c.NewHandler(mux, &http2.Server{}) behavior without the deprecated
// golang.org/x/net/http2/h2c package.
func h2cProtocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}

func (s *service) init(address string) error {
	clientOpts := []micro.ClientOption{micro.WithLogger(s.world.tel.GetLogger("service"))}
	if cfg := s.world.options.NATSConfig; cfg != nil {
		clientOpts = append(clientOpts, micro.WithNATSConfig(*cfg))
	}
	client, err := micro.NewClient(clientOpts...)
	if err != nil {
		return eris.Wrap(err, "failed to initialize micro client")
	}
	s.client = client
	microService, err := micro.NewService(client, s.world.address, &s.world.tel)
	if err != nil {
		return eris.Wrap(err, "failed to create micro service")
	}
	s.microService = microService

	// Keep these for now cuz ISC requires a bit more work than client connections. Will need another
	// refactor after the current clients are migrated to connect directly to the shards.
	commandNames := make([]string, 0, len(s.commands))
	for cmd := range s.commands {
		commandNames = append(commandNames, cmd)
	}
	if err := s.microService.ServeCommands(commandNames, s.Submit); err != nil {
		return eris.Wrap(err, "failed to register inter-shard command handlers")
	}

	mux := http.NewServeMux()

	authMiddleware, err := auth.NewMiddleware(s.authMode, s.argusAuthURL)
	if err != nil {
		return err
	}

	cardinalPath, cardinalHandler, err := s.shardServer.Handler()
	if err != nil {
		return eris.Wrap(err, "failed to create cardinal service handler")
	}
	mux.Handle(cardinalPath, authMiddleware.Wrap(cardinalHandler))

	if err := s.mountDebugService(mux); err != nil {
		return err
	}

	s.server = &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		Protocols:         h2cProtocols(),
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", address)
	if err != nil {
		return eris.Wrap(err, "failed to listen for service server")
	}

	go func() {
		if err := s.server.Serve(listener); err != nil && !eris.Is(err, http.ErrServerClosed) {
			s.log.Error().Err(err).Msg("service server error")
		}
	}()

	return nil
}

func (s *service) mountDebugService(mux *http.ServeMux) error {
	if s.world.debug == nil {
		return nil
	}
	if err := s.world.debug.finalizeCatalog(); err != nil {
		return eris.Wrap(err, "failed to finalize introspection catalog")
	}
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return eris.Wrap(err, "failed to create otel interceptor")
	}
	debugPath, debugHandler := cardinalv1connect.NewDebugServiceHandler(
		s.world.debug,
		connect.WithInterceptors(otelInterceptor, validate.NewInterceptor()),
	)
	mux.Handle(debugPath, debugHandler)
	s.log.Info().Msg("DebugService mounted on client-facing port (dev)")
	return nil
}

func (s *service) shutdown(ctx context.Context) error {
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			return eris.Wrap(err, "failed to shutdown service server")
		}
	}
	if s.microService != nil {
		if err := s.microService.Close(); err != nil {
			return eris.Wrap(err, "failed to close micro service")
		}
	}
	if s.client != nil {
		s.client.Close()
	}

	return nil
}

func (s *service) registerCommandHandler(name string) {
	s.commands[name] = struct{}{}
}

// -------------------------------------------------------------------------------------------------
// Event publishers
// -------------------------------------------------------------------------------------------------

// publishDefaultEvent marshals a world event and hands it to the shared server, which decides who
// receives it. Called from the event manager's dispatch at the end of a tick.
func (s *service) publishDefaultEvent(evt event.Event) error {
	payload, ok := evt.Payload.(event.Payload)
	if !ok {
		return eris.Errorf("invalid event payload type: %T", evt.Payload)
	}

	// Proto via the event's generated MarshalWire (dispatch by type, no registry). payload is an
	// event.Payload (schema.Serializable), so MarshalWire is guaranteed by the type — no fallback.
	payloadPb, err := payload.MarshalWire()
	if err != nil {
		return eris.Wrap(err, "failed to marshal event payload")
	}

	return s.shardServer.PublishEvent(shard.Event{
		Name:      payload.Name(),
		Payload:   payloadPb,
		Recipient: evt.Recipient,
	})
}

// -------------------------------------------------------------------------------------------------
// ISC
// -------------------------------------------------------------------------------------------------

func (s *service) publishInterShardCommand(evt event.Event) error {
	isc, ok := evt.Payload.(command.Command)
	if !ok {
		return eris.Errorf("invalid inter shard command %v", evt.Payload)
	}
	assert.That(isc.Address != nil, "inter shard command has nil address")

	payload, err := isc.Payload.MarshalWire()
	if err != nil {
		// Non-blocking but serious: a dropped shard-to-shard command must not halt the tick, so log at
		// error level and move on rather than propagate (which would surface only as an aggregated warn).
		s.log.Error().Err(err).Str("command", isc.Payload.Name()).Msg("inter-shard command dropped: marshal failed")
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), interShardSendTimeout)
	defer cancel()

	// TODO: revisit shard-to-shard blocking. Dispatch runs synchronously in the tick loop, so this
	// request-reply blocks the whole world up to 10s per send — and we discard the reply anyway. If
	// shard-to-shard isn't meant to block the tick, make this async (worker) or fire-and-forget Publish.
	if err := s.client.SendCommand(ctx, isc.Address, isc.Payload.Name(), isc.Persona, payload); err != nil {
		s.log.Error().Err(err).Str("command", isc.Payload.Name()).Msg("inter-shard command dropped: send failed")
		return nil
	}

	return nil
}
