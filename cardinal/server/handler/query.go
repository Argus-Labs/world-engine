package handler

import (
	"github.com/gofiber/fiber/v2"

	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
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
func PostQuery(world servertypes.ProviderWorld) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		ctx.Set("Content-Type", "application/json")
		resBz, err := world.QueryHandler(ctx.Params("name"), ctx.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusNotFound, "query not found")
		}
		return ctx.Send(resBz)
	}
}
