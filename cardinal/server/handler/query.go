package handler

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// PostQuery godoc
//
//	@Summary      Executes a query
//	@Description  Executes a query
//	@Accept       application/json
//	@Produce      application/json
//	@Param        queryName  path      string  true  "Name of a registered query"
//	@Param        queryBody  body      object  true  "Query to be executed"
//	@Success      200        {object}  object  "Results of the executed query"
//	@Failure      400        {string}  string  "Invalid request parameters"
//	@Router       /query/game/{queryName} [post]
func PostQuery(queries map[string]map[string]engine.Query, wCtx engine.Context) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		query, ok := queries[ctx.Params("group")][ctx.Params("name")]
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "query name not found")
		}

		ctx.Set("Content-Type", "application/json")
		resBz, err := query.HandleQueryRaw(wCtx, ctx.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "encountered an error in query: "+err.Error())
		}

		return ctx.Send(resBz)
	}
}
