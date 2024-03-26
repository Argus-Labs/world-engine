package handler

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type GetEndpointsResponse struct {
	TxEndpoints    []string `json:"txEndpoints"`
	QueryEndpoints []string `json:"queryEndpoints"`
}

// GetEndpoints godoc
//
//	@Summary		Get all http endpoints from Cardinal
//	@Description	Retrieves all http endpoints of the registered messages and queries
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200	{object}	GetEndpointsResponse	"List of http endpoints"
//	@Router			/query/http/endpoints [get]
func GetEndpoints(
	msgs map[string]map[string]types.Message, queries map[string]map[string]engine.Query,
) func(*fiber.Ctx) error {
	// Build the list of /tx/... endpoints
	txEndpoints := make([]string, 0, len(msgs))
	for group, msgMap := range msgs {
		for name := range msgMap {
			txEndpoints = append(txEndpoints, utils.GetTxURL(group, name))
		}
	}

	// Build the list of /query/... endpoints
	queryEndpoints := make([]string, 0, len(queries))
	for group, queryMap := range queries {
		for name := range queryMap {
			queryEndpoints = append(queryEndpoints, utils.GetQueryURL(group, name))
		}
	}

	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(GetEndpointsResponse{
			TxEndpoints:    txEndpoints,
			QueryEndpoints: queryEndpoints,
		})
	}
}
