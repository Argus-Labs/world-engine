package server

import (
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/swagger"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/v2/server/handler"
	"pkg.world.dev/world-engine/cardinal/v2/world"

	_ "pkg.world.dev/world-engine/cardinal/v2/server/docs" // for swagger.
)

const (
	defaultPort     = "4040"
	shutdownTimeout = 5 * time.Second
)

type Server struct {
	app *fiber.App
	w   *world.World
}

// New returns an HTTP server with handlers for all QueryTypes and MessageTypes.
func New(w *world.World) (*Server, error) {
	if w == nil {
		return nil, eris.New("server requires an non-nil world and tick manager")
	}

	app := fiber.New(fiber.Config{
		Network:               "tcp", // Enable server listening on both ipv4 & ipv6 (default: ipv4 only)
		DisableStartupMessage: true,
		ErrorHandler:          ErrorHandler,
	})
	app.Use(cors.New())

	s := &Server{
		app: app,
		w:   w,
	}
	s.setupRoutes()

	return s, nil
}

// Serve serves the application, blocking the calling thread.
// Call this in a new go routine to prevent blocking.
func (s *Server) Serve(ctx context.Context) error {
	serverErr := make(chan error, 1)

	// Starts the server in a new goroutine
	go func() {
		port := os.Getenv("CARDINAL_PORT")
		if port == "" {
			port = defaultPort
		}

		log.Info().Msgf("Starting HTTP server at port %s", port)
		if err := s.app.Listen(":" + port); err != nil {
			serverErr <- eris.Wrap(err, "error starting http server")
		}
	}()

	// This function will block until the server is shutdown or the context is canceled.
	select {
	case err := <-serverErr:
		return eris.Wrap(err, "server encountered an error")
	case <-ctx.Done():
		if err := s.shutdown(); err != nil {
			return eris.Wrap(err, "error shutting down server")
		}
	}

	return nil
}

func (s *Server) BroadcastEvent(event any) error {
	eventBz, err := json.Marshal(event)
	if err != nil {
		return err
	}
	socketio.Broadcast(eventBz)
	return nil
}

// Shutdown gracefully shuts down the server and closes all active websocket connections.
func (s *Server) shutdown() error {
	log.Info().Msg("Shutting down server")

	// Close websocket connections
	socketio.Broadcast([]byte(""), socketio.CloseMessage)
	socketio.Fire(socketio.EventClose, nil)

	// Gracefully shutdown Fiber server
	if err := s.app.ShutdownWithTimeout(shutdownTimeout); err != nil {
		return eris.Wrap(err, "error shutting down server")
	}

	log.Info().Msg("Successfully shut down server")
	return nil
}

// @title		Cardinal
// @description	Backend server for World Engine
// @version		0.0.1
// @schemes		http ws
// @BasePath	/
// @consumes	application/json
// @produces	application/json
func (s *Server) setupRoutes() {
	// Route: /swagger/
	s.app.Get("/swagger/*", swagger.HandlerDefault)

	// Route: /events/
	s.app.Use("/events", handler.WebSocketUpgrader)
	s.app.Get("/events", handler.WebSocketEvents())

	// Route: /world
	s.app.Get("/world", handler.GetWorld(s.w))

	// Route: /...
	s.app.Get("/health", handler.GetHealth())

	// Route: /query/...
	q := s.app.Group("/query")
	q.Post("/receipts/list", handler.GetReceipts(s.w))
	q.Post("/:group/:name", handler.PostQuery(s.w))

	// Route: /tx/...
	tx := s.app.Group("/tx")
	tx.Post("/:group/:name", handler.PostTransaction(s.w))

	// Route: /debug/state
	s.app.Post("/debug/state", handler.GetState(s.w))
}
