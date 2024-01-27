package handler

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

type GetEndpointsResponse struct {
	TxEndpoints    []string `json:"txEndpoints"`
	QueryEndpoints []string `json:"queryEndpoints"`
}

// GetEndpoints godoc
//
//	@Summary		Get all http endpoints from cardinal
//	@Description	Get all http endpoints from cardinal
//	@Accept			application/json
//	@Produce		application/json
//	@Success		200	{object}	GetEndpointsResponse	"list of query endpoints"
//	@Failure		400	{string}	string					"Invalid query request"
//	@Router			/query/http/endpoints [post]
func GetEndpoints(
	msgs map[string]map[string]message.Message, queries map[string]map[string]ecs.Query,
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
