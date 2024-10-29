package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/world"
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
func PostQuery(w *world.World) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		ctx.Set("Content-Type", "application/json")
		resBz, err := w.HandleQuery(ctx.Params("group"), ctx.Params("name"), ctx.Body())
		if eris.Is(err, types.ErrQueryNotFound) {
			return fiber.NewError(fiber.StatusNotFound, "query not found")
		} else if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "encountered an error in query: "+err.Error())
		}
		return ctx.Send(resBz)
	}
}
