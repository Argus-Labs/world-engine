package server

import (
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/events"
)

func handleEvents(hub events.EventHub) (func(*fiber.Ctx) error, func(*fiber.Ctx) error) {
	return events.FiberWebSocketUpgrader, websocket.New(events.CreateWebSocketEventHandler(hub))
}

func (s *Server) registerEventsHandler(path string) {
	websocketUpgrader, websocketHandler := handleEvents(s.eventHub)
	s.app.Use(path, websocketUpgrader)
	s.app.Get(path, websocketHandler)
}
