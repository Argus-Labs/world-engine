package handler

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
)

func WebSocketEvents(wsEventHandler func(conn *websocket.Conn)) func(c *fiber.Ctx) error {
	return websocket.New(wsEventHandler)
}
func WebSocketUpgrader(c *fiber.Ctx) error {
	// IsWebSocketUpgrade returns true if the client
	// requested upgrade to the WebSocket protocol.
	if websocket.IsWebSocketUpgrade(c) {
		c.Locals("allowed", true)
		return eris.Wrap(c.Next(), "")
	}
	return fiber.ErrUpgradeRequired
}
