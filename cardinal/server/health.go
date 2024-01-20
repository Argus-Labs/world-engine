package server

import "github.com/gofiber/fiber/v2"

type HealthReply struct {
	IsServerRunning   bool `json:"isServerRunning"`
	IsGameLoopRunning bool `json:"isGameLoopRunning"`
}

func (s *Server) registerHealthEndpoint(path string) {
	s.app.Get(path, func(c *fiber.Ctx) error {
		res := HealthReply{
			IsServerRunning:   true,
			IsGameLoopRunning: s.eng.IsGameLoopRunning(),
		}
		return c.JSON(res)
	})
}
