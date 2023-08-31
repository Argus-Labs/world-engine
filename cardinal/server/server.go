package server

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/mitchellh/mapstructure"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/shard"

	"github.com/go-openapi/loads"
	swagger_runtime "github.com/go-openapi/runtime"
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

	th := &Handler{
		w:   w,
		mux: http.NewServeMux(),
	}
	for _, opt := range opts {
		opt(th)
	}

	specDoc, err := loads.Spec(pathToSwaggerSpec)
	if err != nil {
		return nil, err
	}
	api := untyped.NewAPI(specDoc).WithoutJSONDefaults()
	api.RegisterConsumer("application/json", swagger_runtime.JSONConsumer())
	api.RegisterProducer("application/json", swagger_runtime.JSONProducer())
	err = registerTxHandlerSwagger(w, api, th)
	if err != nil {
		return nil, err
	}
	err = registerReadHandlerSwagger(w, api, th)
	if err != nil {
		return nil, err
	}

	if err := api.Validate(); err != nil {
		log.Fatalln(err)
	}

	app := middleware.NewContext(specDoc, api, nil)

	th.mux.Handle("/", app.APIHandler(nil))
	th.initialize()

	return th, nil
}

// utility function to create a swagger handler from a request name, request constructor, request to response function.
func createSwaggerHandler[Request any, Response any](requestName string, createRequest func() Request, requestHandler func(*Request) (*Response, error)) swagger_runtime.OperationHandlerFunc {
	return func(params interface{}) (interface{}, error) {
		request := createRequest()
		ok := getValueFromParams[Request](&params, requestName, &request)
		if !ok {
			return nil, errors.New(fmt.Sprintf("could not find %s in parameters", requestName))
		}
		resp, err := requestHandler(&request)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

// utility function to extract parameters from swagger handlers
func getValueFromParams[T any](params *interface{}, name string, value *T) bool {
	data, ok := (*params).(map[string]interface{})
	if !ok {
		return ok
	}
	mappedStructUntyped, ok := data[name]
	if !ok {
		return ok
	}
	mappedStruct, ok := mappedStructUntyped.(map[string]interface{})
	if !ok {
		return ok
	}
	err := mapstructure.Decode(mappedStruct, value)
	if err != nil {
		return ok
	}
	return true
}

// register transaction handlers on swagger server
func registerTxHandlerSwagger(world *ecs.World, api *untyped.API, handler *Handler) error {
	//var txEndpoints []string

	// Register tx endpoints
	txs, err := world.ListTransactions()
	if err != nil {
		return err
	}

	txNameToTx := make(map[string]*transaction.ITransaction)
	for _, tx := range txs {
		txNameToTx[tx.Name()] = &tx
	}

	coreHandler := swagger_runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		//txType, ok := getValueFromParams(&params, "txType")
		//if !ok {
		//	return nil, errors.New("txType not found in params")
		//}
		//txTypeString, ok := txType.(string)
		//if !ok {
		//	return nil, errors.New("txType was not a string")
		//}
		//fmt.Print(txTypeString)
		return TransactionReply{
			TxHash: "",
			Tick:   0,
		}, nil
	})

	createPersonaHandler := swagger_runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return ecs.CreatePersonaTransactionResult{Success: true}, nil
	})

	authorizePersonaAddressHandler := swagger_runtime.OperationHandlerFunc(func(i interface{}) (interface{}, error) {
		return ecs.AuthorizePersonaAddressResult{Success: true}, nil
	})
	api.RegisterOperation("POST", "/tx/core/{txType}", coreHandler)
	api.RegisterOperation("POST", "/tx/persona/create-persona", createPersonaHandler)
	api.RegisterOperation("POST", "/tx/persona/authorize-persona-address", authorizePersonaAddressHandler)

	return nil
}

// result struct for /query/http/endpoints
type EndpointsResult struct {
	TxEndpoints    []string `json:"tx_endpoints"`
	QueryEndpoints []string `json:"query_endpoints"`
}

// register query endpoints for swagger server
func registerReadHandlerSwagger(world *ecs.World, api *untyped.API, handler *Handler) error {
	//var txEndpoints []string
	coreHandler := swagger_runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return struct {
			test string
		}{test: "test"}, nil
	})
	listHandler := swagger_runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		txs, err := world.ListTransactions()
		txEndpoints := make([]string, 0, len(txs))
		if err != nil {
			return nil, err
		}
		for _, tx := range txs {
			if tx.Name() != "create-persona" && tx.Name() != "authorize-persona-address" {
				txEndpoints = append(txEndpoints, "/tx/core/"+tx.Name())
			} else {
				txEndpoints = append(txEndpoints, "/tx/persona/"+tx.Name())
			}
		}

		queryEndpoints := make([]string, 0, len(world.ListReads())+3)
		for _, read := range world.ListReads() {
			queryEndpoints = append(queryEndpoints, "/query/core/"+read.Name())
		}
		queryEndpoints = append(queryEndpoints, "/query/http/endpoints")
		queryEndpoints = append(queryEndpoints, "/query/persona/signer")
		queryEndpoints = append(queryEndpoints, "/query/receipt/submit")
		return EndpointsResult{
			TxEndpoints:    txEndpoints,
			QueryEndpoints: queryEndpoints,
		}, nil
	})
	personaHandler := createSwaggerHandler[ReadPersonaSignerRequest, ReadPersonaSignerResponse](
		"ReadPersonaSignerRequest",
		makeReadPersonaSignerRequest,
		handler.getPersonaSignerResponse)
	receiptsHandler := swagger_runtime.OperationHandlerFunc(func(i interface{}) (interface{}, error) {
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
		}{}}, nil
	})
	api.RegisterOperation("GET", "/query/core/{readType}", coreHandler)
	api.RegisterOperation("GET", "/query/http/endpoints", listHandler)
	api.RegisterOperation("POST", "/query/persona/signer", personaHandler)
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
