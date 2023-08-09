package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/argus-labs/world-engine/cardinal/ecs"
	"github.com/argus-labs/world-engine/cardinal/shard"
)

// Handler is a type that contains endpoints for transactions and queries in a given ecs world.
type Handler struct {
	w                      *ecs.World
	mux                    *http.ServeMux
	server                 *http.Server
	disableSigVerification bool

	// plugins
	adapter shard.WriteAdapter
}

var (
	// ErrorInvalidSignature is returned when a signature is incorrect in some way (e.g. namespace mismatch, nonce invalid,
	// the actual Verify fails). Other failures (e.g. Redis is down) should not wrap this error.
	ErrorInvalidSignature = errors.New("invalid signature")
)

const (
	listTxEndpoint   = "list/tx-endpoints"
	listReadEndpoint = "list/read-endpoints"
	listTxReceipts   = "list/tx-receipts"

	schemaEndpointPrefix = "schema/"

	readPrefix = "read-"
	txPrefix   = "tx-"

	getSignerForPersonaStatusUnknown   = "unknown"
	getSignerForPersonaStatusAvailable = "available"
	getSignerForPersonaStatusAssigned  = "assigned"
)

// NewHandler returns a new Handler that can handle HTTP requests. An HTTP endpoint for each
// transaction and read registered with the given world is automatically created.
func NewHandler(w *ecs.World, opts ...Option) (*Handler, error) {
	th := &Handler{
		w:   w,
		mux: http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(th)
	}

	// register tx endpoints
	if err := registerTxHandlers(w, th); err != nil {
		return nil, fmt.Errorf("failed to register tx handlers: %w", err)
	}

	// register read endpoints
	if err := registerReadHandlers(w, th); err != nil {
		return nil, fmt.Errorf("failed to register read handlers: %w", err)
	}

	if err := registerReceiptEndpoints(w, th); err != nil {
		return nil, fmt.Errorf("failed to register receipt handlers: %w", err)
	}

	return th, nil
}

// Serve sets up the endpoints passed in by the user, as well as a special "/list" endpoint, that informs consumers
// what endpoints the user set up in the Handler. Then, it serves the application, blocking the main thread.
// Please us `go txh.Serve(host,port)` if you do not want to block execution after calling this function.
func (t *Handler) Serve(host, port string) {
	t.server = &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: t.mux,
	}
	err := t.server.ListenAndServe()
	if err != nil {
		log.Print(err)
	}
}

func (t *Handler) Close() error {
	if err := t.server.Close(); err != nil {
		return err
	}
	return nil
}
