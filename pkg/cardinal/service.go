package cardinal

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/otelconnect"
	"connectrpc.com/validate"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/transport"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	"github.com/rotisserie/eris"
)

// -------------------------------------------------------------------------------------------------
// Transport wiring
// -------------------------------------------------------------------------------------------------

// newTransport builds the transport from w.options and registers every command registered so far.
func (w *World) newTransport() (*transport.Transport, error) {
	tr, err := transport.New(transport.Options{
		Address:   w.address,
		AuthMode:  w.options.AuthMode,
		ArgusURL:  w.options.ArgusAuthURL,
		Telemetry: &w.tel,
	})
	if err != nil {
		return nil, err
	}
	for _, name := range w.commands.Names() {
		tr.Handle(name, w.commands.Enqueue)
	}
	return tr, nil
}

// natsClient returns the world's NATS connection, opening it on first use. Everything in the world that
// talks to NATS shares it, and shutdown closes it after all of them have stopped.
func (w *World) natsClient() (*micro.Client, error) {
	if w.client != nil {
		return w.client, nil
	}
	opts := []micro.ClientOption{micro.WithLogger(w.tel.GetLogger("nats"))}
	if cfg := w.options.NATSConfig; cfg != nil {
		opts = append(opts, micro.WithNATSConfig(*cfg))
	}
	client, err := micro.NewClient(opts...)
	if err != nil {
		return nil, eris.Wrap(err, "failed to connect to NATS")
	}
	w.client = client
	return client, nil
}

// startTransport freezes the introspection catalog, starts the transport on the world's NATS connection,
// and serves it, with the debug service when enabled, on listenAddr.
func (w *World) startTransport(listenAddr string) error {
	if err := w.debug.finalizeCatalog(); err != nil {
		return eris.Wrap(err, "failed to finalize introspection catalog")
	}
	client, err := w.natsClient()
	if err != nil {
		return err
	}
	if err := w.transport.Start(client); err != nil {
		return err
	}

	path, handler, err := w.transport.Handler()
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.Handle(path, handler)
	if w.debug != nil {
		otelInterceptor, err := otelconnect.NewInterceptor()
		if err != nil {
			return eris.Wrap(err, "failed to create otel interceptor")
		}
		mux.Handle(w.debugServiceHandler(connect.WithInterceptors(otelInterceptor, validate.NewInterceptor())))
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", listenAddr)
	if err != nil {
		return eris.Wrap(err, "failed to listen for service server")
	}
	w.server = &http.Server{
		Addr:              listener.Addr().String(), // The bound address, so a ":0" port can be found
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		Protocols:         h2cProtocols(),
	}
	go func() {
		if err := w.server.Serve(listener); err != nil && !eris.Is(err, http.ErrServerClosed) {
			w.tel.Logger.Error().Err(err).Msg("service server error")
		}
	}()
	return nil
}

// stopTransport stops the transport, then the HTTP server serving it. The transport goes first: it ends
// open event streams, which would otherwise hold the server's shutdown until ctx expires.
func (w *World) stopTransport(ctx context.Context) error {
	err := w.transport.Stop(ctx)
	if w.server != nil {
		if shutdownErr := w.server.Shutdown(ctx); shutdownErr != nil {
			err = errors.Join(err, eris.Wrap(shutdownErr, "failed to shutdown service server"))
		}
	}
	return err
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

// debugServiceHandler builds the DebugService handler. It reads w.debug when the transport starts.
func (w *World) debugServiceHandler(opts ...connect.HandlerOption) (string, http.Handler) {
	w.tel.Logger.Info().Msg("DebugService mounted on client-facing port (dev)")
	return cardinalv1connect.NewDebugServiceHandler(w.debug, opts...)
}

func (w *World) publishEvent(ctx context.Context, evt event.Event) error {
	payload, ok := evt.Payload.(event.Payload)
	if !ok {
		return eris.Errorf("invalid event payload type: %T", evt.Payload)
	}
	w.transport.Publish(ctx, payload, evt.Recipient)
	return nil
}

func (w *World) sendInterShardCommand(ctx context.Context, evt event.Event) error {
	isc, ok := evt.Payload.(command.Command)
	if !ok {
		return eris.Errorf("invalid inter shard command %v", evt.Payload)
	}
	w.transport.Enqueue(ctx, isc.Address, isc.Payload)
	return nil
}

// -------------------------------------------------------------------------------------------------
// Authentication
// -------------------------------------------------------------------------------------------------

type (
	User     = transport.User
	AuthMode = transport.AuthMode
)

const (
	AuthModeUndefined = transport.AuthModeUndefined
	AuthModeArgus     = transport.AuthModeArgus
	AuthModeDev       = transport.AuthModeDev
)

func ParseAuthMode(s string) (AuthMode, error) {
	return transport.ParseAuthMode(s)
}

func UserFromContext(ctx context.Context) *User {
	return transport.UserFromContext(ctx)
}
