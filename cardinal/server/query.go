package server

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/runtime/middleware"
	"github.com/go-openapi/runtime/middleware/untyped"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	"pkg.world.dev/world-engine/cardinal/ecs/query"
	"pkg.world.dev/world-engine/cardinal/world_namespace"
)

// register query endpoints for swagger server
func (handler *Handler) registerReadHandlerSwagger(api *untyped.API) error {
	readNameToReadType := make(map[string]ecs.IRead)
	for _, read := range handler.w.ListReads() {
		readNameToReadType[read.Name()] = read
	}

	// query/game/{readType} is a dynamic route that must dynamically handle things thus it can't use
	// the createSwaggerQueryHandler utility function below as the Request and Reply types are dynamic.
	queryHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
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
		rawJsonReply, err := outputType.HandleReadRaw(handler.w, rawJsonBody)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(rawJsonReply), nil

	})
	endpoints, err := createAllEndpoints(handler.w)
	if err != nil {
		return err
	}
	listHandler := runtime.OperationHandlerFunc(func(params interface{}) (interface{}, error) {
		return endpoints, nil
	})

	personaHandler := createSwaggerQueryHandler[ReadPersonaSignerRequest, ReadPersonaSignerResponse](
		"ReadPersonaSignerRequest",
		handler.getPersonaSignerResponse)

	receiptsHandler := createSwaggerQueryHandler[ListTxReceiptsRequest, ListTxReceiptsReply](
		"ListTxReceiptsRequest",
		getListTxReceiptsReplyFromRequest(handler.w),
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
		resultFilter, err := cql.CQLParse(cqlString, handler.w.GetComponentByName)
		if err != nil {
			return middleware.Error(422, err), nil
		}

		result := make([]cql.QueryResponse, 0)

		query.NewQuery(resultFilter).Each(world_namespace.Namespace(handler.w.Namespace()), handler.w.Store(), func(id entity.ID) bool {
			components, err := handler.w.StoreManager().GetComponentTypesForEntity(id)
			if err != nil {
				return false
			}
			resultElement := cql.QueryResponse{
				id,
				make([]json.RawMessage, 0),
			}

			for _, c := range components {
				var getter component.IGettableRawJsonFromEntityId
				getter, ok = c.(component.IGettableRawJsonFromEntityId)
				if !ok {
					err = fmt.Errorf("%s is not serializeable to json", c.Name())
					return false
				}
				var data json.RawMessage
				data, err = getter.GetRawJson(handler.w.StoreManager(), id)
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
	api.RegisterOperation("POST", "/query/game/{readType}", queryHandler)
	api.RegisterOperation("POST", "/query/http/endpoints", listHandler)
	api.RegisterOperation("POST", "/query/persona/signer", personaHandler)
	api.RegisterOperation("POST", "/query/receipts/list", receiptsHandler)

	return nil
}
