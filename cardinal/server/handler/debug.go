package handler

import (
	"github.com/gofiber/fiber/v2"

	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
)

// GetDebugState godoc
//
// @Summary      Retrieves a list of all entities in the game state
// @Description  Retrieves a list of all entities in the game state
// @Produce      application/json
// @Success      200  {object}  DebugStateResponse "List of all entities"
// @Router       /debug/state [post]
func GetDebugState(provider servertypes.ProviderWorld) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		result, eachClosureErr, searchEachErr := provider.GetDebugState()
		if eachClosureErr != nil {
			return eachClosureErr
		}
		if searchEachErr != nil {
			return searchEachErr
		}
		return ctx.JSON(&result)
	}
}
