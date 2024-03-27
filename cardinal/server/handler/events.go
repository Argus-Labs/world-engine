package handler

import (
	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// WebSocketEvents godoc
//
//	@Summary      Establishes a new websocket connection to retrieve system events
//	@Description  Establishes a new websocket connection to retrieve system events
//	@Produce      application/json
//	@Success      101  {string}  string  "Switch protocol to ws"
//	@Router       /events [get]
func WebSocketEvents() func(c *fiber.Ctx) error {
	return socketio.New(func(_ *socketio.Websocket) {
		log.Debug().Msg("new websocket connection established")
	})
}

func WebSocketUpgrader(c *fiber.Ctx) error {
	// IsWebSocketUpgrade returns true if the client
	// requested upgrade to the WebSocket protocol.
	if websocket.IsWebSocketUpgrade(c) {
		c.Locals("allowed", true)
		return c.Next()
	}
	return fiber.ErrUpgradeRequired
}
