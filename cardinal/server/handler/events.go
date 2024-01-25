package handler

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/events"
)

func WebSocketEvents(hub events.EventHub) (func(*fiber.Ctx) error, func(*fiber.Ctx) error) {
	return events.FiberWebSocketUpgrader, websocket.New(events.CreateWebSocketEventHandler(hub))
}
