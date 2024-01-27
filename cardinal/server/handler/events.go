package handler

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// WebSocketEvents godoc
//
//	@Summary		Endpoint for events
//	@Description	websocket connection for events.
//	@Produce		application/json
//	@Success		101	{string}	string	"switch protocol to ws"
//	@Router			/events [get]
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
