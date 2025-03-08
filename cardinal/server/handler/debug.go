package handler

import (
	"github.com/gofiber/fiber/v2"

	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"
)

type DebugStateRequest struct{}

type DebugStateResponse = []types.DebugStateElement

// @Summary Get the debug state of the world.
// @Description Get the debug state of the world.
// @Accept json
// @Produce json
// @Param request body DebugStateRequest true "Debug state request"
// @Success 200 {array} types.DebugStateElement
// @Failure 500 {object} error
// @Router /debug/state [post].
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
