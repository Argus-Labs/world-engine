package server3

import "github.com/gofiber/fiber/v2"

type HealthReply struct {
	IsServerRunning   bool `json:"isServerRunning"`
	IsGameLoopRunning bool `json:"isGameLoopRunning"`
}

func (s *Server) registerHealthHandler() {
	s.app.Get("/health", func(c *fiber.Ctx) error {
		res := HealthReply{
			IsServerRunning:   true,
			IsGameLoopRunning: s.eng.IsGameLoopRunning(), // Adapt this to your actual game loop check
		}
		return c.JSON(res)
	})
}
