package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/sign"

	"github.com/go-openapi/loads"
	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/runtime/middleware/untyped"
	"github.com/mitchellh/mapstructure"
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
	getSignerForPersonaStatusUnknown   = "unknown"
	getSignerForPersonaStatusAvailable = "available"
	getSignerForPersonaStatusAssigned  = "assigned"
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
	err = registerTxHandlerSwagger(w, api, th)
	if err != nil {
		return nil, err
	}
	err = registerReadHandlerSwagger(w, api, th)
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

func getTxFromParams(pathParam string, params interface{}, txNameToTx map[string]transaction.ITransaction) (transaction.ITransaction, error) {
	mappedParams, ok := params.(map[string]interface{})
	if !ok {
		return nil, errors.New("params not readable")
	}
	txType, ok := mappedParams[pathParam]
	if !ok {
		return nil, errors.New("params do not contain txType from the path /tx/game/{txType}")
	}
	txTypeString, ok := txType.(string)
	if !ok {
		return nil, errors.New("txType needs to be a string from path")
	}
	tx, ok := txNameToTx[txTypeString]
	if !ok {
		return nil, errors.New(fmt.Sprintf("could not locate transaction type: %s", txTypeString))
	}
	return tx, nil
}

func getBodyAndSigFromParams(
	params interface{},
	handler *Handler, isSystemTransaction bool) ([]byte, *sign.SignedPayload, error) {
	mappedParams, ok := params.(map[string]interface{})
	if !ok {
		return nil, nil, errors.New("params not readable")
	}
	txBody, ok := mappedParams["txBody"]
	if !ok {
		return nil, nil, errors.New("params do not contain txBody from the body of the http request")
	}
	txBodyMap, ok := txBody.(map[string]interface{})
	if !ok {
		return nil, nil, errors.New("txBody needs to be a json object in the body")

	}
	payload, sp, err := handler.verifySignatureOfMapRequest(txBodyMap, isSystemTransaction)
	if err != nil {
		return nil, nil, err
	}
	return payload, sp, nil
}

func processTxBodyMap(tx transaction.ITransaction, payload []byte, sp *sign.SignedPayload, handler *Handler) (*TransactionReply, error) {
	rawJsonResult, err := handler.processTransaction(tx, payload, sp)
	transactionReply := new(TransactionReply)
	err = json.Unmarshal(rawJsonResult, transactionReply)
	if err != nil {
		return nil, err
	}
	return transactionReply, nil
}

// register transaction handlers on swagger server
func registerTxHandlerSwagger(world *ecs.World, api *untyped.API, handler *Handler) error {
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
		payload, sp, err := getBodyAndSigFromParams(params, handler, false)
		if err != nil {
			return nil, err
		}
		tx, err := getTxFromParams("txType", params, txNameToTx)
		if err != nil {
			return middleware.Error(404, err), nil
		}
		if tx.Name() == ecs.AuthorizePersonaAddressTx.Name() {
			return nil, fmt.Errorf("This route should not process %s, use tx/persona/%s", tx.Name(), ecs.AuthorizePersonaAddressTx.Name())
		}
		return processTxBodyMap(tx, payload, sp, handler)
	})

	// will be moved to ecs
	createPersonaHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		payload, sp, err := getBodyAndSigFromParams(params, handler, true)
		if err != nil {
			if errors.Is(err, ErrorInvalidSignature) || errors.Is(err, ErrorSystemTransactionRequired) {
				return middleware.Error(401, err), nil
			} else {
				return nil, err
			}
		}
		if err != nil {
			return nil, err
		}
		txReply, err := generateCreatePersonaResponseFromPayload(payload, sp, ecs.CreatePersonaTx, world)
		if err != nil {
			return nil, err
		}
		return &txReply, nil
	})

	// will be moved to ecs
	authorizePersonaAddressHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		rawPayload, signedPayload, err := getBodyAndSigFromParams(params, handler, false)
		if err != nil {
			return nil, err
		}
		return processTxBodyMap(ecs.AuthorizePersonaAddressTx, rawPayload, signedPayload, handler)
	})
	api.RegisterOperation("POST", "/tx/game/{txType}", gameHandler)
	api.RegisterOperation("POST", "/tx/persona/create-persona", createPersonaHandler)
	api.RegisterOperation("POST", "/tx/persona/authorize-persona-address", authorizePersonaAddressHandler)

	return nil
}

// EndpointsResult result struct for /query/http/endpoints
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
	queryEndpoints = append(queryEndpoints, "/query/receipt/list")
	queryEndpoints = append(queryEndpoints, "/query/game/cql")
	return &EndpointsResult{
		TxEndpoints:    txEndpoints,
		QueryEndpoints: queryEndpoints,
	}, nil
}

// register query endpoints for swagger server
func registerReadHandlerSwagger(world *ecs.World, api *untyped.API, handler *Handler) error {
	//var txEndpoints []string
	readNameToReadType := make(map[string]ecs.IRead)
	for _, read := range world.ListReads() {
		readNameToReadType[read.Name()] = read
	}

	// query/game/{readType} is a dynamic route that must dynamically handle things thus it can't use
	// the createSwaggerQueryHandler utility function below as the Request and Reply types are dynamic.
	gameHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		mapStruct, ok := params.(map[string]interface{})
		if !ok {
			return nil, errors.New("invalid parameter input, map could not be created")
		}
		readTypeUntyped, ok := mapStruct["readType"]
		if !ok {
			return nil, errors.New("readType parameter not found")
		}
		readTypeString, ok := readTypeUntyped.(string)
		if !ok {
			return nil, fmt.Errorf("readType was the wrong type, it should be a string from the path")
		}
		outputType, ok := readNameToReadType[readTypeString]
		if !ok {
			return middleware.Error(404, fmt.Errorf("readType of type %s does not exist", readTypeString)), nil
		}

		bodyData, ok := mapStruct["readBody"]
		if !ok {
			return nil, errors.New("readBody parameter not found")
		}
		bodyDataAsMap, ok := bodyData.(map[string]interface{})
		if !ok {
			return nil, errors.New("data not convertable to map")
		}

		//Huge hack.
		//the json body comes in as a map.
		//go-swagger validates all the data and shoves it into a map
		//I can't get the relevant Request Type associated with the Read here
		//So I convert that map into raw json
		//Then I have IRead.HandleReadRaw just output a rawJsonReply.
		//I convert that into a json.RawMessage which go-swagger will validate.
		rawJsonBody, err := json.Marshal(bodyDataAsMap)
		if err != nil {
			return nil, err
		}
		rawJsonReply, err := outputType.HandleReadRaw(world, rawJsonBody)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(rawJsonReply), nil

	})
	endpoints, err := createAllEndpoints(world)
	if err != nil {
		return err
	}
	listHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return endpoints, nil
	})

	// Will be moved to ecs.
	personaHandler := createSwaggerQueryHandler[ReadPersonaSignerRequest, ReadPersonaSignerResponse](
		"ReadPersonaSignerRequest",
		handler.getPersonaSignerResponse)

	receiptsHandler := createSwaggerQueryHandler[ListTxReceiptsRequest, ListTxReceiptsReply](
		"ListTxReceiptsRequest",
		getListTxReceiptsReplyFromRequest(world),
	)

	cqlHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		mapStruct, ok := params.(map[string]interface{})
		if !ok {
			return nil, errors.New("invalid parameter input, map could not be created")
		}
		cqlRequestUntyped, ok := mapStruct["cql"]
		if !ok {
			return nil, errors.New("cql body parameter could not be found")
		}
		cqlRequest, ok := cqlRequestUntyped.(map[string]interface{})
		if !ok {
			return middleware.Error(422, fmt.Errorf("json is invalid")), nil
		}
		cqlStringUntyped, ok := cqlRequest["CQL"]
		if !ok {
			return middleware.Error(422, fmt.Errorf("json is invalid")), nil
		}
		cqlString, ok := cqlStringUntyped.(string)
		if !ok {
			return middleware.Error(422, fmt.Errorf("json is invalid")), nil
		}
		resultFilter, err := cql.CQLParse(cqlString, world.GetComponentByName)
		if err != nil {
			return middleware.Error(422, err), nil
		}

		result := make([]cql.QueryResponse, 0)

		ecs.NewQuery(resultFilter).Each(world, func(id entity.ID) bool {
			components, err := world.StoreManager().GetComponentTypesForEntity(id)
			if err != nil {
				return false
			}
			resultElement := cql.QueryResponse{
				id,
				make([]json.RawMessage, 0),
			}

			for _, c := range components {
				hasJSON, ok := c.(ecs.GettableAsJSON)
				if !ok {
					err = errors.New("GetAsJSON method not valid on this component")
					return false
				}
				var data json.RawMessage
				data, err = hasJSON.GetAsJSON(world, id)
				if err != nil {
					return false
				}

				resultElement.Data = append(resultElement.Data, data)
			}
			result = append(result, resultElement)
			return true
		})
		if err != nil {
			return nil, err
		}

		return result, nil
	})

	api.RegisterOperation("POST", "/query/game/cql", cqlHandler)
	api.RegisterOperation("POST", "/query/game/{readType}", gameHandler)
	api.RegisterOperation("POST", "/query/http/endpoints", listHandler)
	api.RegisterOperation("POST", "/query/persona/signer", personaHandler)
	api.RegisterOperation("POST", "/query/receipts/list", receiptsHandler)

	return nil
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
	err := t.server.ListenAndServe()
	return err
}

func (t *Handler) Close() error {
	return t.server.Close()
}

func (t *Handler) Shutdown() error {
	ctx := context.Background()
	return t.server.Shutdown(ctx)
}
