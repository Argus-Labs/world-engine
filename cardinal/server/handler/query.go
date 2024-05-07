package handler

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/server/types"
)

// PostQuery godoc
//
//	@Summary      Executes a query
//	@Description  Executes a query
//	@Accept       application/json
//	@Produce      application/json
//	@Param        queryGroup  path      string  true  "Query group"
//	@Param        queryName   path      string  true  "Name of a registered query"
//	@Param        queryBody   body      object  true  "Query to be executed"
//	@Success      200         {object}  object  "Results of the executed query"
//	@Failure      400         {string}  string  "Invalid request parameters"
//	@Router       /query/{queryGroup}/{queryName} [post]
func PostQuery(queries map[string]map[string]types.ProviderQuery, wCtx types.ProviderContext) func(*fiber.Ctx) error {
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

// NOTE: duplication for cleaner swagger docs
// PostQuery godoc
//
//	@Summary      Executes a query
//	@Description  Executes a query
//	@Accept       application/json
//	@Produce      application/json
//	@Param        queryName   path      string  true  "Name of a registered query"
//	@Param        queryBody   body      object  true  "Query to be executed"
//	@Success      200         {object}  object  "Results of the executed query"
//	@Failure      400         {string}  string  "Invalid request parameters"
//	@Router       /query/game/{queryName} [post]
func PostGameQuery(queries map[string]map[string]types.ProviderQuery, wCtx types.ProviderContext) func(*fiber.Ctx) error {
	return PostQuery(queries, wCtx)
}
