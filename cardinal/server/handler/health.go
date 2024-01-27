package handler

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type GetHealthResponse struct {
	IsServerRunning   bool `json:"isServerRunning"`
	IsGameLoopRunning bool `json:"isGameLoopRunning"`
}

// GetHealth godoc
//
//	@Summary		Get information on status of world-engine
//	@Description	Displays information on http server and world game loop
//	@Produce		application/json
//	@Success		200	{object}	GetHealthResponse
//	@Router			/health [get]
func GetHealth(engine *ecs.Engine) func(c *fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(GetHealthResponse{
			IsServerRunning:   true,
			IsGameLoopRunning: engine.IsGameLoopRunning(),
		})
	}
}
