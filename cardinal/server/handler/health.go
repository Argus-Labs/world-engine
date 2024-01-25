package handler

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type GetHealthResponse struct {
	IsServerRunning   bool `json:"isServerRunning"`
	IsGameLoopRunning bool `json:"isGameLoopRunning"`
}

func GetHealth(engine *ecs.Engine) func(c *fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(GetHealthResponse{
			IsServerRunning:   true,
			IsGameLoopRunning: engine.IsGameLoopRunning(),
		})
	}
}
