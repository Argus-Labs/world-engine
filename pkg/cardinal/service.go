package cardinal

import (
	"context"
	"net/http"

	"connectrpc.com/connect"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
	"github.com/argus-labs/world-engine/pkg/cardinal/internal/event"
	"github.com/argus-labs/world-engine/pkg/transport"
	"github.com/argus-labs/world-engine/proto/gen/go/worldengine/cardinal/v1/cardinalv1connect"
	"github.com/rotisserie/eris"
)

// -------------------------------------------------------------------------------------------------
// Transport wiring
// -------------------------------------------------------------------------------------------------

// startTransport freezes the introspection catalog, then starts serving clients and other shards.
func (w *World) startTransport(address string) error {
	if err := w.debug.finalizeCatalog(); err != nil {
		return eris.Wrap(err, "failed to finalize introspection catalog")
	}
	return w.transport.Start(address)
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
