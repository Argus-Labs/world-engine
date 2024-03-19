package handler

import (
	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
)

// WebSocketEvents godoc
//
//	@Summary		Endpoint for events
//	@Description	websocket connection for events.
//	@Produce		application/json
//	@Success		101	{string}	string	"switch protocol to ws"
//	@Router			/events [get]
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
