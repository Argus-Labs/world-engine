// Package transport carries a shard's commands and events: the client-facing ConnectRPC service and
// shard-to-shard commands over NATS. It knows nothing about how the shard processes them.
package transport

import (
	"context"
	"net/http"

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

// Handler receives one command addressed to this shard, from a client or another shard. sender is the
// authenticated player for a client, the sending shard for a shard. A returned error rejects the
// command back to its sender.
type Handler func(ctx context.Context, cmd *iscv1.Command, sender Sender) error

// Payload is the encoding half of a generated command or event type.
type Payload interface {
	Name() string
	SizeWire() int
	AppendWire(b []byte) []byte
}

// Wire is a generated command or event type: its encoding and its decoding.
type Wire interface {
	Payload
	UnmarshalWire(b []byte) (any, error)
}

type Options struct {
	Address      *micro.ServiceAddress // This shard's address
	AuthMode     AuthMode              // Authentication for client requests
	ArgusURL     string                // Argus Auth service URL; required when AuthMode is ARGUS
	Organization string                // Game tokens' organization; required when AuthMode is ARGUS
	Project      string                // Game tokens' project; required when AuthMode is ARGUS
	Telemetry    *telemetry.Telemetry
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

// Transport serves one shard's client and shard-to-shard traffic. It owns neither connection: the app
// serves Handler on its own HTTP server and passes its NATS client to Start, opening both before Start
// and closing them after Stop.
type Transport struct {
	opts     Options
	log      zerolog.Logger
	handlers map[string]Handler // Written before Handler or Start, read-only after
	sealed   bool               // Set by Handler and Start; Handle panics afterwards

	clients      *clientService
	inflight     inflight
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

// Handle registers h for the named command. It panics after Handler or Start, or when name is already
// handled.
func (t *Transport) Handle(name string, h Handler) {
	if t.sealed {
		panic(eris.Errorf("transport: Handle(%q) called after Handler or Start", name))
	}
	if _, exists := t.handlers[name]; exists {
		panic(eris.Errorf("transport: command %q is already handled", name))
	}
	t.handlers[name] = h
}

// Command is one decoded command addressed to this service.
type Command[T Wire] struct {
	Payload T
	Sender  Sender // The authenticated player for a client, the sending shard for a shard
}

// RegisterCommand registers h for commands named T.Name(). The transport decodes each payload into T
// before calling h; a payload that does not decode is rejected back to its sender like a handler error.
// Registering through T also declares it as a command this service receives, which is how the SDK
// generator finds the types to generate wire code for. It panics as Handle does.
func (t *Transport) RegisterCommand[T Wire](h func(ctx context.Context, cmd Command[T]) error) {
	var zero T
	name := zero.Name()
	t.Handle(name, func(ctx context.Context, raw *iscv1.Command, sender Sender) error {
		decoded, err := zero.UnmarshalWire(raw.GetPayload())
		if err != nil {
			return eris.Wrapf(err, "failed to decode command %s", name)
		}
		payload, ok := decoded.(T)
		if !ok {
			return eris.Errorf("command %s decoded to %T, not %T", name, decoded, zero)
		}
		return h(ctx, Command[T]{Payload: payload, Sender: sender})
	})
}

// RegisterEvent declares T as an event or result this service publishes, so the SDK generator emits its
// wire code. It is only a declaration: Publish takes any Payload and does not check it, since a result
// whose name carries a request ID is only named at runtime. It panics after Handler or Start, like Handle.
func (t *Transport) RegisterEvent[T Wire]() {
	if t.sealed {
		var zero T
		panic(eris.Errorf("transport: RegisterEvent[%T] called after Handler or Start", zero))
	}
}

// errStopping rejects commands that arrive once Stop has begun.
var errStopping = eris.New("shard is shutting down")

func (t *Transport) dispatch(ctx context.Context, cmd *iscv1.Command, sender Sender) error {
	h, ok := t.handlers[cmd.GetName()]
	if !ok {
		return eris.Errorf("unregistered command: %s", cmd.GetName())
	}
	if !t.inflight.enter() {
		return errStopping
	}
	defer t.inflight.exit()
	return h(ctx, cmd, sender)
}

// Handler returns CardinalService's path and handler, with authentication, tracing and request
// validation applied, for the app to serve on its own HTTP server.
func (t *Transport) Handler() (string, http.Handler, error) {
	t.sealed = true

	otelInterceptor, err := otelconnect.NewInterceptor()
	if err != nil {
		return "", nil, eris.Wrap(err, "failed to create otel interceptor")
	}
	interceptors := connect.WithInterceptors(otelInterceptor, validate.NewInterceptor())

	var authenticate func(context.Context, *http.Request) (any, error)
	switch t.opts.AuthMode {
	case AuthModeArgus:
		authenticator, err := NewArgusAuthenticator(t.opts.ArgusURL, t.opts.Organization, t.opts.Project)
		if err != nil {
			return "", nil, eris.Wrap(err, "failed to create argus authenticator")
		}
		authenticate = authenticator.Authenticate
	case AuthModeDev:
		authenticate = authenticatorDev{}.authenticate
	case AuthModeUndefined:
		fallthrough
	default:
		return "", nil, eris.Errorf("invalid service auth mode: %s", t.opts.AuthMode)
	}

	path, handler := cardinalv1connect.NewCardinalServiceHandler(t.clients, interceptors)
	return path, authn.NewMiddleware(authenticate).Wrap(handler), nil
}

// Start registers the ping endpoint and one endpoint per handled command on client. Commands can arrive
// as soon as it returns. The caller keeps client open until after Stop, and closes it.
func (t *Transport) Start(client *micro.Client) error {
	if client == nil {
		return eris.New("NATS client is required")
	}
	if t.interShard != nil {
		return eris.New("transport already started")
	}
	t.sealed = true

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

// Stop ends this transport's work, waiting until ctx at most, so the app can then shut down its HTTP
// server and close the NATS client:
//  1. New commands are rejected as unavailable, and the NATS endpoints are unsubscribed.
//  2. Handlers already running finish. A service that handles commands concurrently may Enqueue and
//     Flush replies from them, so they must finish before step 3.
//  3. Flushed commands are sent. Commands flushed after this are dropped and logged.
//  4. Open event streams and reply waits end as unavailable, so the HTTP server's shutdown does not
//     wait on them until ctx expires.
//
// It is safe to call without Start, or after Start failed partway.
func (t *Transport) Stop(ctx context.Context) error {
	t.inflight.close()
	var err error
	if t.microService != nil {
		if closeErr := t.microService.Close(); closeErr != nil {
			err = eris.Wrap(closeErr, "failed to unsubscribe NATS endpoints")
		}
	}
	if !t.inflight.wait(ctx) {
		t.log.Error().Msg("command handlers still running at the shutdown deadline")
	}
	if t.interShard != nil {
		t.interShard.stop(ctx)
	}
	t.clients.stop()
	return err
}

// Publish delivers an event to the open event streams subscribed to it and to the reply waiters for
// its name. An empty recipient means every subscribed stream; otherwise only that user's stream.
func (t *Transport) Publish(ctx context.Context, evt Payload, recipient string) {
	t.clients.publish(ctx, evt, recipient)
}

// Enqueue stages cmd for the shard at to, with this shard's address as its sender. Nothing is sent
// until Flush. Enqueue and Flush are safe to call from any goroutine, but only after Start.
func (t *Transport) Enqueue(ctx context.Context, to *micro.ServiceAddress, cmd Payload) {
	assert.That(to != nil, "inter shard command has nil address")
	assert.That(t.interShard != nil, "inter shard command enqueued before the transport started")

	t.interShard.enqueue(ctx, &iscv1.InterShardCommand{
		Command: &iscv1.Command{
			Name:    cmd.Name(),
			Address: to,
			Payload: cmd.AppendWire(make([]byte, 0, cmd.SizeWire())),
		},
		Sender: t.opts.Address,
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
