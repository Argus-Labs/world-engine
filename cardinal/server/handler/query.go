package handler

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// PostQuery godoc
//
//	@Summary      Run a query on Cardinal
//	@Description  Runs a registered query on Cardinal and returns the results
//	@Accept       application/json
//	@Produce      application/json
//	@Param        queryName  path      string  true  "Name of the registered query"
//	@Param        queryBody  body      object  true  "Query body"
//	@Success      200        {object}  object  "Query results"
//	@Failure      400        {string}  string  "Invalid request body or invalid query body"
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
