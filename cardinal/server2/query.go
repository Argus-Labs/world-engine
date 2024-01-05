package server

import (
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/ecs"
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
	listHandler := func(c *fiber.Ctx) error {
		return c.JSON(endpoints)
	}

	personaHandler := createQueryHandlerFromRequest[QueryPersonaSignerRequest, QueryPersonaSignerResponse](
		"QueryPersonaSignerRequest",
		handler.getPersonaSignerResponse,
	)

	receiptsHandler := createQueryHandlerFromRequest[ListTxReceiptsRequest, ListTxReceiptsReply](
		"ListTxReceiptsRequest",
		getListTxReceiptsReplyFromRequest(handler.w),
	)

	handler.server.Post("/query/game/{queryType}", queryHandler)
	handler.server.Post("/query/http/endpoints", listHandler)
	handler.server.Post("/query/persona/signer", personaHandler)
	handler.server.Post("/query/receipts/list", receiptsHandler)
	return nil
}
