package handler

import (
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type HealthReply struct {
	IsServerRunning   bool `json:"isServerRunning"`
	IsGameLoopRunning bool `json:"isGameLoopRunning"`
}

func GetHealth(eng *ecs.Engine) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		return c.JSON(HealthReply{
			IsServerRunning:   true,
			IsGameLoopRunning: eng.IsGameLoopRunning(),
		})
	}
}
