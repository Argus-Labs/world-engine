package handler

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/world"
)

type GetWorldResponse = types.WorldInfo

// GetWorld godoc
//
//	@Summary      Retrieves details of the game world
//	@Description  Contains the registered components, messages, queries, and namespace
//	@Accept       application/json
//	@Produce      application/json
//	@Success      200  {object}  types.WorldInfo   "Details of the game world"
//	@Failure      400  {string}  string            "Invalid request parameters"
//	@Router       /world [get]
func GetWorld(w *world.World) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		queries := w.RegisteredQuries()
		queryInfo := make([]types.EndpointInfo, 0, len(queries))
		for _, q := range queries {
			queryInfo = append(queryInfo, types.EndpointInfo{
				Name:   q.Name(),
				Fields: q.GetRequestFieldInformation(),
				URL:    utils.GetQueryURL(q.Group(), q.Name()),
			})
		}

		return ctx.JSON(&types.WorldInfo{
			Namespace:  w.Namespace(),
			Components: w.State().RegisteredComponents(),
			Messages:   w.RegisteredMessages(),
			Queries:    queryInfo,
		})
	}
}
