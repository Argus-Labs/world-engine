package handler

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

func PostQuery(queries map[string]ecs.Query, eng *ecs.Engine, wildcard string) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		query, exists := queries[ctx.Params(wildcard)]
		if !exists {
			return fiber.NewError(fiber.StatusNotFound, "no query type found")
		}
		return handleQuery(ctx, eng, query)
	}
}

func PostCustomPathQuery(query ecs.Query, eng *ecs.Engine) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		return handleQuery(ctx, eng, query)
	}
}

func handleQuery(ctx *fiber.Ctx, eng *ecs.Engine, query ecs.Query) error {
	resBz, err := query.HandleQueryRaw(ecs.NewReadOnlyEngineContext(eng), ctx.Body())
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "encountered an error in query: "+err.Error())
	}
	ctx.Set("Content-Type", "application/json")
	return ctx.Send(resBz)
}
