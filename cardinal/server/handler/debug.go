package handler

import (
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/world"
)

type DebugStateRequest struct{}

type DebugStateResponse = []types.EntityData

// GetState godoc
//
// @Summary      Retrieves a list of all entities in the game state
// @Description  Retrieves a list of all entities in the game state
// @Produce      application/json
// @Success      200  {object}  DebugStateResponse "List of all entities"
// @Router       /debug/state [post]
func GetState(w *world.World) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		entities := make([]types.EntityData, 0)

		var eachErr error
		err := w.Search(filter.All()).Each(func(id types.EntityID) bool {
			components, err := w.State().FinalizedState().GetAllComponentsForEntityInRawJSON(id)
			if err != nil {
				eachErr = err
				return false
			}

			entities = append(entities, types.EntityData{
				ID:         id,
				Components: components,
			})

			return true
		})
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if eachErr != nil {
			return fiber.NewError(fiber.StatusInternalServerError, eachErr.Error())
		}

		return ctx.JSON(entities)
	}
}
