package handler

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// PostQuery godoc
//
//	@Summary		Query the ecs
//	@Description	Query the ecs
//	@Accept			application/json
//	@Produce		application/json
//	@Param			queryType	path		string	true	"The query type"
//	@Param			queryBody	body		object	true	"Query Message"
//	@Success		200			{object}	object	"query response"
//	@Failure		400			{string}	string	"Invalid query request"
//	@Router			/query/game/{queryType} [post]
func PostQuery(queries map[string]map[string]engine.Query, wCtx engine.Context) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		query, ok := queries[ctx.Params("group")][ctx.Params("name")]
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "query type not found")
		}

		ctx.Set("Content-Type", "application/json")
		resBz, err := query.HandleQueryRaw(wCtx, ctx.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "encountered an error in query: "+err.Error())
		}

		return ctx.Send(resBz)
	}
}
