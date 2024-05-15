package handler

import (
	"github.com/gofiber/fiber/v2"

	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"
)

type DebugStateResponse = []types.EntityStateElement

// GetState godoc
//
// @Summary      Retrieves a list of all entities in the game state
// @Description  Retrieves a list of all entities in the game state
// @Produce      application/json
// @Success      200  {object}  DebugStateResponse "List of all entities"
// @Router       /debug/state [post]
func GetState(world servertypes.ProviderWorld) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		var result DebugStateResponse
		result, err := world.GetDebugState()
		if err != nil {
			return err
		}

		return ctx.JSON(&result)
	}
}
