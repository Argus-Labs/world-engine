package server

import (
	"encoding/json"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/cql"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

// register query endpoints for swagger app.
func (handler *Handler) registerQueryHandlers() error {
	queryHandler := func(c *fiber.Ctx) error {
		queryType := c.Params("{queryType}")
		if queryType == "" {
			return fiber.NewError(fiber.StatusBadRequest, "queryType was not found in the params")
		}
		q, err := handler.w.GetQueryByName(queryType)
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, fmt.Sprintf("query %s not found", queryType))
		}
		queryBody := c.Body()
		wCtx := ecs.NewReadOnlyEngineContext(handler.w)
		rawJSONReply, err := q.HandleQueryRaw(wCtx, queryBody)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, eris.ToString(err, true))
		}
		// rawJSONReply is already a JSON-formatted []byte so we can write it directly to response
		c.Set("Content-Type", "application/json")
		return c.Send(rawJSONReply)
	}

	endpoints, err := createAllEndpoints(handler.w)
	if err != nil {
		return err
	}
	getEndpointsListHandler := func(c *fiber.Ctx) error {
		return c.JSON(endpoints)
	}

	getPersonaSignerHandler := createQueryHandlerFromRequest[QueryPersonaSignerRequest, QueryPersonaSignerResponse](
		handler.getPersonaSignerResponse,
	)

	getReceiptsListHandler := createQueryHandlerFromRequest[ListTxReceiptsRequest, ListTxReceiptsReply](
		getListTxReceiptsReplyFromRequest(handler.w),
	)

	cqlHandler := func(c *fiber.Ctx) error {
		requestBody := c.Body()

		var cqlString string
		if len(requestBody) != 0 {
			request := new(cql.QueryRequest)
			err := json.Unmarshal(requestBody, request)
			if err != nil {
				return fiber.NewError(fiber.StatusBadRequest, eris.Wrapf(err, "unable to unmarshal query request into type %T", *request).Error())
			}
			cqlString = request.CQL
		} else {
			return fiber.NewError(fiber.StatusBadRequest, "body in CQL query request was empty")
		}
		resultFilter, err := cql.Parse(cqlString, handler.w.GetComponentByName)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}

		result := make([]cql.QueryResponse, 0)

		wCtx := ecs.NewReadOnlyEngineContext(handler.w)
		err = ecs.NewSearch(resultFilter).Each(
			wCtx, func(id entity.ID) bool {
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
			},
		)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf("unable to perform Search for CQL query: %s", err))
		}

		return c.JSON(result)
	}

	// Note: /query/game/cql must be registered before /query/game/:{queryType} because the latter would catch the
	// former's requests due to the wildcard parameter otherwise
	handler.app.Post("/query/game/cql", cqlHandler)
	handler.app.Post("/query/game/:{queryType}", queryHandler)
	handler.app.Post("/query/http/endpoints", getEndpointsListHandler)
	handler.app.Post("/query/persona/signer", getPersonaSignerHandler)
	handler.app.Post("/query/receipts/list", getReceiptsListHandler)
	return nil
}
