package server

import "github.com/gofiber/fiber/v2"

type HealthReply struct {
	IsServerRunning   bool `json:"isServerRunning"`
	IsGameLoopRunning bool `json:"isGameLoopRunning"`
}

func (handler *Handler) registerHealthHandler() {
	handler.app.Get("/health", func(c *fiber.Ctx) error {
		res := HealthReply{
			IsServerRunning:   true,
			IsGameLoopRunning: handler.w.IsGameLoopRunning(), // Adapt this to your actual game loop check
		}

		return c.JSON(res)
	})
}
