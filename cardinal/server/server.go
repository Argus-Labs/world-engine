package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/shard"
	"strconv"
)

// Handler is a type that contains endpoints for transactions and queries in a given ecs world.
type Handler struct {
	w                      *ecs.World
	mux                    *http.ServeMux
	server                 *http.Server
	disableSigVerification bool
	port                   string

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

	// Don't name this tx-receipts to ensure it doesn't match the prefix for normal transactions
	txReceiptsEndpoint = "transaction-receipts"

	schemaEndpointPrefix = "schema/"

	readPrefix = "read-"
	txPrefix   = "tx-"

	getSignerForPersonaStatusUnknown   = "unknown"
	getSignerForPersonaStatusAvailable = "available"
	getSignerForPersonaStatusAssigned  = "assigned"
)

// NewHandler returns a new Handler that can handle HTTP requests. An HTTP endpoint for each
// transaction and read registered with the given world is automatically created. The server runs on a default port
// of 4040, but can be changed via options or by setting an environment variable with key CARDINAL_PORT.
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
	th.initialize()
	return th, nil
}

// initialize initializes the server. It firsts checks for a port set on the handler via options.
// if no port is found, or a bad port was passed into the option, it falls back to an environment variable,
// CARDINAL_PORT. If not set, it falls back to a default port of 4040.
func (t *Handler) initialize() {
	if _, err := strconv.Atoi(t.port); err != nil || len(t.port) == 0 {
		envPort := os.Getenv("CARDINAL_PORT")
		if _, err := strconv.Atoi(envPort); err == nil {
			t.port = envPort
		} else {
			t.port = "4040"
		}
	}
	t.server = &http.Server{
		Addr:    fmt.Sprintf(":%s", t.port),
		Handler: t.mux,
	}
}

// Serve sets up the endpoints passed in by the user, as well as a special "/list" endpoint, that informs consumers
// what endpoints the user set up in the Handler. Then, it serves the application, blocking the main thread.
// Please us `go txh.Serve()` if you do not want to block execution after calling this function.
// Will default to env var "CARDINAL_PORT". If that's not set correctly then will default to port 4040
// if no correct port was previously set.
func (t *Handler) Serve() {
	err := t.server.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}

func (t *Handler) Close() error {
	if err := t.server.Close(); err != nil {
		return err
	}
	return nil
}
