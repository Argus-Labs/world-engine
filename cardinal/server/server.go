package server

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/shard"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/runtime/middleware/untyped"
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

// NewSwaggerHandler instantiates handler function for creating a swagger server that validates itself based on a swagger spec.
func NewSwaggerHandler(w *ecs.World, pathToSwaggerSpec string, opts ...Option) (*Handler, error) {
	specDoc, err := loads.Spec(pathToSwaggerSpec)
	if err != nil {
		return nil, err
	}
	api := untyped.NewAPI(specDoc)
	err = registerTxHandlerSwagger(w, api)
	if err != nil {
		return nil, err
	}
	err = registerReadHandlerSwagger(w, api)
	if err != nil {
		return nil, err
	}

	if err := api.Validate(); err != nil {
		return nil, err
	}

	app := middleware.NewContext(specDoc, api, nil)
	th := &Handler{
		w:   w,
		mux: http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(th)
	}
	th.mux.Handle("/", app.APIHandler(nil))
	th.initialize()
	return th, nil
}

// utility function to extract parameters from swagger handlers
func getValueFromParams(params interface{}, name string) (interface{}, bool) {
	data, ok := (params).(map[string]interface{})
	if !ok {
		return nil, ok
	}
	return data[name], true
}

// register transaction handlers on swagger server
func registerTxHandlerSwagger(world *ecs.World, api *untyped.API) error {
	//var txEndpoints []string

	// Register tx endpoints
	txs, err := world.ListTransactions()
	if err != nil {
		return err
	}

	txNameToTx := make(map[string]transaction.ITransaction)
	for _, tx := range txs {
		txNameToTx[tx.Name()] = tx
	}

	gameHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return TransactionReply{
			TxHash: "",
			Tick:   0,
		}, errors.New("not implemented")
	})

	// will be moved to ecs
	createPersonaHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return ecs.CreatePersonaTransactionResult{Success: true}, errors.New("not implemented")
	})

	// will be moved to ecs
	authorizePersonaAddressHandler := runtime.OperationHandlerFunc(func(i interface{}) (interface{}, error) {
		return ecs.AuthorizePersonaAddressResult{Success: true}, errors.New("not implemented")
	})
	api.RegisterOperation("POST", "/tx/game/{txType}", gameHandler)
	api.RegisterOperation("POST", "/tx/persona/create-persona", createPersonaHandler)
	api.RegisterOperation("POST", "/tx/persona/authorize-persona-address", authorizePersonaAddressHandler)

	return nil
}

// result struct for /query/http/endpoints
type EndpointsResult struct {
	TxEndpoints    []string `json:"tx_endpoints"`
	QueryEndpoints []string `json:"query_endpoints"`
}

func createAllEndpoints(world *ecs.World) (*EndpointsResult, error) {
	txs, err := world.ListTransactions()
	txEndpoints := make([]string, 0, len(txs))
	if err != nil {
		return nil, err
	}
	for _, tx := range txs {
		if tx.Name() != ecs.CreatePersonaTx.Name() && tx.Name() != ecs.AuthorizePersonaAddressTx.Name() {
			txEndpoints = append(txEndpoints, "/tx/game/"+tx.Name())
		} else {
			txEndpoints = append(txEndpoints, "/tx/persona/"+tx.Name())
		}
	}

	queryEndpoints := make([]string, 0, len(world.ListReads())+3)
	for _, read := range world.ListReads() {
		queryEndpoints = append(queryEndpoints, "/query/game/"+read.Name())
	}
	queryEndpoints = append(queryEndpoints, "/query/http/endpoints")
	queryEndpoints = append(queryEndpoints, "/query/persona/signer")
	queryEndpoints = append(queryEndpoints, "/query/receipt/submit")
	return &EndpointsResult{
		TxEndpoints:    txEndpoints,
		QueryEndpoints: queryEndpoints,
	}, nil
}

// register query endpoints for swagger server
func registerReadHandlerSwagger(world *ecs.World, api *untyped.API) error {
	//var txEndpoints []string
	gameHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return struct {
			test string
		}{test: "test"}, errors.New("not implemented")
	})
	endpoints, err := createAllEndpoints(world)
	if err != nil {
		return err
	}
	listHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return endpoints, nil
	})

	// Will be moved to ecs.
	personaHandler := runtime.OperationHandlerFunc(func(i interface{}) (interface{}, error) {
		return ReadPersonaSignerResponse{Status: "", SignerAddress: ""}, errors.New("not implemented")
	})
	receiptsHandler := runtime.OperationHandlerFunc(func(i interface{}) (interface{}, error) {
		return struct {
			start_tick uint64
			end_tick   uint64
			Receipt    []struct {
				tx_hash string
				tick    uint64
				result  interface{}
				errors  []string
			}
		}{start_tick: 0, end_tick: 0, Receipt: []struct {
			tx_hash string
			tick    uint64
			result  interface{}
			errors  []string
		}{}}, errors.New("not implemented")
	})
	api.RegisterOperation("GET", "/query/game/{readType}", gameHandler)
	api.RegisterOperation("GET", "/query/http/endpoints", listHandler)
	api.RegisterOperation("GET", "/query/persona/signer", personaHandler)
	api.RegisterOperation("GET", "/query/receipts/submit", receiptsHandler)

	return nil
}

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
func (t *Handler) Serve() error {
	return t.server.ListenAndServe()
}

func (t *Handler) Close() error {
	return t.server.Close()
}
