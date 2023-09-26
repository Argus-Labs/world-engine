package server

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/runtime/middleware/untyped"
	"github.com/mitchellh/mapstructure"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/shard"
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

// NewHandler instantiates handler function for creating a swagger server that validates itself based on a swagger spec.
// transaction and read registered with the given world is automatically created. The server runs on a default port
// of 4040, but can be changed via options or by setting an environment variable with key CARDINAL_PORT.
func NewHandler(w *ecs.World, opts ...Option) (*Handler, error) {
	h, err := newSwaggerHandlerEmbed(w, opts...)
	if err != nil {
		return nil, err
	}
	return h, nil
}

//go:embed swagger.yml
var swaggerData []byte

func newSwaggerHandlerEmbed(w *ecs.World, opts ...Option) (*Handler, error) {
	th := &Handler{
		w:   w,
		mux: http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(th)
	}
	specDoc, err := loads.Analyzed(swaggerData, "")
	if err != nil {
		return nil, err
	}
	api := untyped.NewAPI(specDoc).WithoutJSONDefaults()
	api.RegisterConsumer("application/json", runtime.JSONConsumer())
	api.RegisterProducer("application/json", runtime.JSONProducer())
	err = th.registerTxHandlerSwagger(api)
	if err != nil {
		return nil, err
	}
	err = th.registerReadHandlerSwagger(api)
	if err != nil {
		return nil, err
	}
	err = th.registerHealthHandlerSwagger(api)
	if err != nil {
		return nil, err
	}

	if err := api.Validate(); err != nil {
		return nil, err
	}

	app := middleware.NewContext(specDoc, api, nil)

	th.mux.Handle("/", app.APIHandler(nil))
	th.initialize()

	return th, nil
}

// utility function to create a swagger handler from a request name, request constructor, request to response function.
func createSwaggerQueryHandler[Request any, Response any](requestName string, requestHandler func(*Request) (*Response, error)) runtime.OperationHandlerFunc {
	return func(params interface{}) (interface{}, error) {
		request, ok := getValueFromParams[Request](params, requestName)
		if !ok {
			return middleware.Error(404, fmt.Errorf("%s not found", requestName)), nil
		}
		resp, err := requestHandler(request)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

// utility function to extract parameters from swagger handlers
func getValueFromParams[T any](params interface{}, name string) (*T, bool) {
	data, ok := params.(map[string]interface{})
	if !ok {
		return nil, ok
	}
	mappedStructUntyped, ok := data[name]
	if !ok {
		return nil, ok
	}
	mappedStruct, ok := mappedStructUntyped.(map[string]interface{})
	if !ok {
		return nil, ok
	}
	value := new(T)
	err := mapstructure.Decode(mappedStruct, value)
	if err != nil {
		return nil, ok
	}
	return value, true
}

// EndpointsResult result struct for /query/http/endpoints
type EndpointsResult struct {
	TxEndpoints    []string `json:"tx_endpoints"`
	QueryEndpoints []string `json:"query_endpoints"`
}

func createAllEndpoints(world *ecs.World) (*EndpointsResult, error) {
	txs, err := world.ListTransactions()
	if err != nil {
		return nil, err
	}
	txEndpoints := make([]string, 0, len(txs))
	for _, tx := range txs {
		if tx.Name() != ecs.CreatePersonaTx.Name() && tx.Name() != ecs.AuthorizePersonaAddressTx.Name() {
			txEndpoints = append(txEndpoints, "/tx/game/"+tx.Name())
		} else {
			txEndpoints = append(txEndpoints, "/tx/persona/"+tx.Name())
		}
	}

	reads := world.ListReads()
	queryEndpoints := make([]string, 0, len(reads)+3)
	for _, read := range reads {
		queryEndpoints = append(queryEndpoints, "/query/game/"+read.Name())
	}
	queryEndpoints = append(queryEndpoints, "/query/http/endpoints")
	queryEndpoints = append(queryEndpoints, "/query/persona/signer")
	queryEndpoints = append(queryEndpoints, "/query/receipt/list")
	queryEndpoints = append(queryEndpoints, "/query/game/cql")
	return &EndpointsResult{
		TxEndpoints:    txEndpoints,
		QueryEndpoints: queryEndpoints,
	}, nil
}

// initialize initializes the server. It firsts checks for a port set on the handler via options.
// if no port is found, or a bad port was passed into the option, it falls back to an environment variable,
// CARDINAL_PORT. If not set, it falls back to a default port of 4040.
func (handler *Handler) initialize() {
	if _, err := strconv.Atoi(handler.port); err != nil || len(handler.port) == 0 {
		envPort := os.Getenv("CARDINAL_PORT")
		if _, err := strconv.Atoi(envPort); err == nil {
			handler.port = envPort
		} else {
			handler.port = "4040"
		}
	}
	handler.server = &http.Server{
		Addr:    fmt.Sprintf(":%s", handler.port),
		Handler: handler.mux,
	}
}

// Serve serves the application, blocking the calling thread.
// Call this in a new go routine to prevent blocking.
func (handler *Handler) Serve() error {
	err := handler.server.ListenAndServe()
	return err
}

func (handler *Handler) Close() error {
	return handler.server.Close()
}

func (handler *Handler) Shutdown() error {
	ctx := context.Background()
	return handler.server.Shutdown(ctx)
}
