// Package transport carries a shard's commands and events: the client-facing ConnectRPC service and
// shard-to-shard commands over NATS. It knows nothing about how the shard processes them.
package transport

import (
	"context"
	"net"
	"net/http"
	"time"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"
	"github.com/argus-labs/world-engine/pkg/assert"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
)

// Handler receives one command addressed to this shard, from a client or another shard. The persona
// is already stamped: the authenticated user's ID for a client, the sending shard's address for a
// shard. A returned error rejects the command back to its sender.
type Handler func(ctx context.Context, cmd *iscv1.Command) error

// ServiceHandler builds an extra ConnectRPC service to serve next to CardinalService. It has the shape
// of the generated New<Service>Handler constructors, and gets the same interceptors.
type ServiceHandler func(opts ...connect.HandlerOption) (string, http.Handler)

// Payload is the encoding half of a generated command or event type.
type Payload interface {
	Name() string
	SizeWire() int
	AppendWire(b []byte) []byte
}

type Options struct {
	Address   *micro.ServiceAddress // This shard's address
	AuthMode  AuthMode              // Authentication for client requests
	ArgusURL  string                // Argus Auth service URL; required when AuthMode is ARGUS
	NATS      *micro.NATSConfig     // Nil uses the client's defaults
	Telemetry *telemetry.Telemetry
	Services  []ServiceHandler // Served without authentication, as the debug service is
}

func (o Options) validate() error {
	if o.Address == nil {
		return eris.New("address is required")
	}
	if !o.AuthMode.IsValid() {
		return eris.Errorf("invalid auth mode: %s", o.AuthMode)
	}
	if o.AuthMode == AuthModeArgus && o.ArgusURL == "" {
		return eris.New("argus URL is required when auth mode is ARGUS")
	}
	if o.Telemetry == nil {
		return eris.New("telemetry is required")
	}
	return nil
}

// Transport serves one shard's client and shard-to-shard traffic.
type Transport struct {
	opts     Options
	log      zerolog.Logger
	handlers map[string]Handler // Written before Start, read-only after
	started  bool

	clients      *clientService
	server       *http.Server
	client       *micro.Client
	microService *micro.Service
	interShard   *interShard
}

// New validates opts and returns a Transport that does nothing until Start.
func New(opts Options) (*Transport, error) {
	if err := opts.validate(); err != nil {
		return nil, eris.Wrap(err, "invalid transport options")
	}
	t := &Transport{
		opts:     opts,
		log:      opts.Telemetry.GetLogger("service"),
		handlers: make(map[string]Handler),
	}
	t.clients = newClientService(opts.Address, t.dispatch, t.log)
	return t, nil
}

// Handle registers h for the named command. It panics after Start or when name is already handled.
func (t *Transport) Handle(name string, h Handler) {
	if t.started {
		panic(eris.Errorf("transport: Handle(%q) called after Start", name))
	}
	if _, exists := t.handlers[name]; exists {
		panic(eris.Errorf("transport: command %q is already handled", name))
	}
	t.handlers[name] = h
}

func (t *Transport) dispatch(ctx context.Context, cmd *iscv1.Command) error {
	h, ok := t.handlers[cmd.GetName()]
	if !ok {
		return eris.Errorf("unregistered command: %s", cmd.GetName())
	}
	return h(ctx, cmd)
}

// Start connects to NATS, registers an endpoint per handled command, and serves ConnectRPC on
// listenAddr. Commands can arrive as soon as it returns.
func (t *Transport) Start(listenAddr string) error {
	if t.started {
		return eris.New("transport already started")
	}
	t.started = true

	if err := t.startNATS(); err != nil {
		return err
	}
	return t.startHTTP(listenAddr)
}

// startNATS connects to NATS and registers the ping endpoint and one endpoint per handled command.
func (t *Transport) startNATS() error {
	clientOpts := []micro.ClientOption{micro.WithLogger(t.log)}
	if cfg := t.opts.NATS; cfg != nil {
		clientOpts = append(clientOpts, micro.WithNATSConfig(*cfg))
	}
	client, err := micro.NewClient(clientOpts...)
	if err != nil {
		return eris.Wrap(err, "failed to initialize micro client")
	}
	t.client = client
	microService, err := micro.NewService(client, t.opts.Address, t.opts.Telemetry)
	if err != nil {
		return eris.Wrap(err, "failed to create micro service")
	}
	t.microService = microService
	t.interShard = newInterShard(t.opts.Address, client, t.dispatch, t.log)

	if err := t.microService.AddEndpoint("ping", handlePing); err != nil {
		return eris.Wrap(err, "failed to register ping handler")
	}
	if err := t.interShard.start(t.microService, t.handlers); err != nil {
		return err
	}
	// Subscribing only buffers the request; flush so the server has every endpoint before Start returns.
	if err := client.Flush(); err != nil {
		return eris.Wrap(err, "failed to flush NATS subscriptions")
	}
	return nil
}

// startHTTP serves CardinalService and the extra services on listenAddr.
func (t *Transport) startHTTP(listenAddr string) error {
	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return eris.Wrap(err, "failed to create otel interceptor")
	}
	interceptors := connect.WithInterceptors(otelInterceptor, validate.NewInterceptor())

	var authenticate func(context.Context, *http.Request) (any, error)
	switch t.opts.AuthMode {
	case AuthModeArgus:
		authenticator, err := newAuthenticatorArgus(t.opts.ArgusURL)
		if err != nil {
			return eris.Wrap(err, "failed to create argus authenticator")
		}
		authenticate = authenticator.authenticate
	case AuthModeDev:
		authenticate = authenticatorDev{}.authenticate
	case AuthModeUndefined:
		fallthrough
	default:
		return eris.Errorf("invalid service auth mode: %s", t.opts.AuthMode)
	}

	mux := http.NewServeMux()
	cardinalPath, cardinalHandler := cardinalv1connect.NewCardinalServiceHandler(t.clients, interceptors)
	mux.Handle(cardinalPath, authn.NewMiddleware(authenticate).Wrap(cardinalHandler))
	for _, service := range t.opts.Services {
		path, handler := service(interceptors)
		mux.Handle(path, handler)
	}

	t.server = &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		Protocols:         h2cProtocols(),
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", listenAddr)
	if err != nil {
		return eris.Wrap(err, "failed to listen for service server")
	}

	go func() {
		if err := t.server.Serve(listener); err != nil && !eris.Is(err, http.ErrServerClosed) {
			t.log.Error().Err(err).Msg("service server error")
		}
	}()

	return nil
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

// Stop waits until ctx for flushed commands to be sent, then stops serving and closes NATS. It is safe
// to call without Start, or after Start failed partway.
func (t *Transport) Stop(ctx context.Context) error {
	// Before server.Shutdown: an open event stream holds it until ctx expires, and its error returns early.
	if t.interShard != nil {
		t.interShard.stop(ctx)
	}
	if t.server != nil {
		if err := t.server.Shutdown(ctx); err != nil {
			return eris.Wrap(err, "failed to shutdown service server")
		}
	}
	if t.microService != nil {
		if err := t.microService.Close(); err != nil {
			return eris.Wrap(err, "failed to close micro service")
		}
	}
	if t.client != nil {
		t.client.Close()
	}
	return nil
}

// Publish delivers an event to the open event streams subscribed to it and to the reply waiters for
// its name. An empty recipient means every subscribed stream; otherwise only that user's stream.
func (t *Transport) Publish(ctx context.Context, evt Payload, recipient string) {
	t.clients.publish(ctx, evt, recipient)
}

// Enqueue stages cmd for the shard at to, with this shard's address as its persona. Nothing is sent
// until Flush. Enqueue and Flush must be called from one goroutine, and only after Start.
func (t *Transport) Enqueue(ctx context.Context, to *micro.ServiceAddress, cmd Payload) {
	assert.That(to != nil, "inter shard command has nil address")
	assert.That(t.interShard != nil, "inter shard command enqueued before the transport started")

	t.interShard.enqueue(ctx, &iscv1.Command{
		Name:    cmd.Name(),
		Address: to,
		Persona: &iscv1.Persona{Id: micro.String(t.opts.Address)},
		Payload: cmd.AppendWire(make([]byte, 0, cmd.SizeWire())),
	})
}

// Flush hands every staged command to its target's sender and returns without waiting for the sends.
// It does nothing before Start.
func (t *Transport) Flush() {
	if t.interShard != nil {
		t.interShard.drain()
	}
}

func handlePing(_ context.Context, req *micro.Request) *micro.Response {
	return micro.NewSuccessResponse(req, nil)
}
