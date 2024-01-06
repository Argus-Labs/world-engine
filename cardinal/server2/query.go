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

// register query endpoints for swagger server.
//
//nolint:funlen,gocognit
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
		wCtx := ecs.NewReadOnlyWorldContext(handler.w)
		rawJSONReply, err := q.HandleQueryRaw(wCtx, queryBody)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, eris.ToString(err, true))
		}
		return c.JSON(rawJSONReply)
	}

	endpoints, err := createAllEndpoints(handler.w)
	if err != nil {
		return err
	}
	getEndpointsListHandler := func(c *fiber.Ctx) error {
		return c.JSON(endpoints)
	}

	getPersonaSignerHandler := createQueryHandlerFromRequest[QueryPersonaSignerRequest, QueryPersonaSignerResponse](
		"QueryPersonaSignerRequest",
		handler.getPersonaSignerResponse,
	)

	getReceiptsListHandler := createQueryHandlerFromRequest[ListTxReceiptsRequest, ListTxReceiptsReply](
		"ListTxReceiptsRequest",
		getListTxReceiptsReplyFromRequest(handler.w),
	)

	cqlHandler := func(c *fiber.Ctx) error {
		requestBody := c.Body()

		var cqlString string
		if len(requestBody) != 0 {
			// TODO: Might need to do c.Body(), unmarshall, then grab `CQL` from that obj, check in tests
			if err := c.BodyParser(&cqlString); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("body in CQL query request did not match expected type: %s", err))
			}
		} else {
			return fiber.NewError(fiber.StatusBadRequest, "body in CQL query request was empty")
		}
		resultFilter, err := cql.Parse(cqlString, handler.w.GetComponentByName)
		if err != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
		}

		result := make([]cql.QueryResponse, 0)

		wCtx := ecs.NewReadOnlyWorldContext(handler.w)
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

		// TODO: Check if this matches expected response type using tests
		return c.JSON(result)
	}

	handler.server.Post("/query/game/{queryType}", queryHandler)
	handler.server.Post("/query/game/cql", cqlHandler)
	handler.server.Post("/query/http/endpoints", getEndpointsListHandler)
	handler.server.Post("/query/persona/signer", getPersonaSignerHandler)
	handler.server.Post("/query/receipts/list", getReceiptsListHandler)
	return nil
}
