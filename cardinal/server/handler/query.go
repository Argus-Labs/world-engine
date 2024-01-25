package handler

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

func PostQuery(queries map[string]map[string]ecs.Query, engine *ecs.Engine) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		query, ok := queries[ctx.Params("group")][ctx.Params("name")]
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "query type not found")
		}

		ctx.Set("Content-Type", "application/json")
		resBz, err := query.HandleQueryRaw(ecs.NewReadOnlyEngineContext(engine), ctx.Body())
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "encountered an error in query: "+err.Error())
		}

		return ctx.Send(resBz)
	}
}
