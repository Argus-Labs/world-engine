package handler

import (
	"github.com/gofiber/fiber/v2"
)

type GetHealthResponse struct {
	IsServerRunning   bool `json:"isServerRunning"`
	IsGameLoopRunning bool `json:"isGameLoopRunning"`
}

// GetHealth godoc
//
//	@Summary      Retrieves the status of the server and game loop
//	@Description  Retrieves the status of the server and game loop
//	@Produce      application/json
//	@Success      200  {object}  GetHealthResponse  "Server and game loop status"
//	@Router       /health [get]
func GetHealth() func(c *fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		return ctx.JSON(GetHealthResponse{
			IsServerRunning: true,
			// TODO(scott): reconsider whether we need this. Intuitively server running implies game loop running.
			IsGameLoopRunning: true,
		})
	}
}
