package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/runtime/middleware/untyped"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

// register query endpoints for swagger server.
//
//nolint:funlen,gocognit
func (handler *Handler) registerQueryHandlerSwagger(api *untyped.API) error {
	// query/game/{queryType} is a dynamic route that must dynamically handle things thus it can't use
	// the createSwaggerQueryHandler utility function below as the Request and Reply types are dynamic.
	queryHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		mapStruct, ok := params.(map[string]interface{})
		if !ok {
			return nil, eris.New("invalid parameter input, map could not be created")
		}
		queryTypeUntyped, ok := mapStruct["queryType"]
		if !ok {
			return nil, eris.New("queryType parameter not found")
		}
		queryTypeString, ok := queryTypeUntyped.(string)
		if !ok {
			return nil, eris.Errorf(
				"queryType was the wrong type, it should be a string from the path",
			)
		}

		q, err := handler.w.GetQueryByName(queryTypeString)
		if err != nil {
			return middleware.Error(
				http.StatusNotFound,
				eris.Errorf("query %s not found", queryTypeString),
			), nil //lint:ignore nilerr this is a middleware error that should 404
		}

		bodyData, ok := mapStruct["queryBody"]
		if !ok {
			return nil, eris.New("queryBody parameter not found")
		}
		bodyDataAsMap, ok := bodyData.(map[string]interface{})
		if !ok {
			return nil, eris.New("data not convertable to map")
		}

		// Huge hack.
		// the json body comes in as a map.
		// go-swagger validates all the data and shoves it into a map
		// I can't get the relevant Request Type associated with the Search here
		// So I convert that map into raw json
		// Then I have Query.HandleQueryRaw just output a rawJSONReply.
		// I convert that into a json.RawMessage which go-swagger will validate.
		rawJSONBody, err := json.Marshal(bodyDataAsMap)
		if err != nil {
			return nil, eris.Wrap(err, "could not unmarshal data into map")
		}
		wCtx := ecs.NewReadOnlyWorldContext(handler.w)
		rawJSONReply, err := q.HandleQueryRaw(wCtx, rawJSONBody)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(rawJSONReply), nil
	})
	endpoints, err := createAllEndpoints(handler.w)
	if err != nil {
		return err
	}
	listHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return endpoints, nil
	})

	personaHandler := createSwaggerQueryHandler[QueryPersonaSignerRequest, QueryPersonaSignerResponse](
		"QueryPersonaSignerRequest",
		handler.getPersonaSignerResponse,
	)

	receiptsHandler := createSwaggerQueryHandler[ListTxReceiptsRequest, ListTxReceiptsReply](
		"ListTxReceiptsRequest",
		getListTxReceiptsReplyFromRequest(handler.w),
	)

	cqlHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		mapStruct, ok := params.(map[string]interface{})
		if !ok {
			return nil, eris.New("invalid parameter input, map could not be created")
		}
		cqlRequestUntyped, ok := mapStruct["cql"]
		if !ok {
			return nil, eris.New("cql body parameter could not be found")
		}
		cqlRequest, ok := cqlRequestUntyped.(map[string]interface{})
		if !ok {
			return middleware.Error(
				http.StatusUnprocessableEntity,
				eris.Errorf("json is invalid"),
			), nil
		}
		cqlStringUntyped, ok := cqlRequest["CQL"]
		if !ok {
			return middleware.Error(
				http.StatusUnprocessableEntity,
				eris.Errorf("json is invalid"),
			), nil
		}
		cqlString, ok := cqlStringUntyped.(string)
		if !ok {
			return middleware.Error(
				http.StatusUnprocessableEntity,
				eris.Errorf("json is invalid"),
			), nil
		}
		resultFilter, err := cql.Parse(cqlString, handler.w.GetComponentByName)
		if err != nil {
			return middleware.Error(http.StatusUnprocessableEntity, err), nil
		}

		result := make([]cql.QueryResponse, 0)

		wCtx := ecs.NewReadOnlyWorldContext(handler.w)
		err = ecs.NewSearch(resultFilter).Each(wCtx, func(id entity.ID) bool {
			components, err := wCtx.StoreReader().GetComponentTypesForEntity(id)
			if err != nil {
				return false
			}
			resultElement := cql.QueryResponse{
				ID:   id,
				Data: make([]json.RawMessage, 0),
			}

			for _, c := range components {
				data, err := wCtx.StoreReader().GetComponentForEntityInRawJSON(c, id)
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
	api.RegisterOperation("POST", "/query/game/{queryType}", queryHandler)
	api.RegisterOperation("POST", "/query/http/endpoints", listHandler)
	api.RegisterOperation("POST", "/query/persona/signer", personaHandler)
	api.RegisterOperation("POST", "/query/receipts/list", receiptsHandler)

	return nil
}
