package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/swagger"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/server/handler"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"

	_ "pkg.world.dev/world-engine/cardinal/server/docs" // for swagger.
)

const (
	defaultPort     = "4040"
	shutdownTimeout = 5 * time.Second
)

type config struct {
	port                            string
	isSignatureVerificationDisabled bool
	isSwaggerDisabled               bool
}

type Server struct {
	app    *fiber.App
	config config
}

// New returns an HTTP server with handlers for all QueryTypes and MessageTypes.
func New(
	world servertypes.ProviderWorld,
	components []types.ComponentMetadata,
	messages []types.Message,
	opts ...Option,
) (*Server, error) {
	app := fiber.New(fiber.Config{
		Network:               "tcp", // Enable server listening on both ipv4 & ipv6 (default: ipv4 only)
		DisableStartupMessage: true,
	})

	s := &Server{
		app: app,
		config: config{
			port:                            defaultPort,
			isSignatureVerificationDisabled: false,
			isSwaggerDisabled:               false,
		},
	}
	for _, opt := range opts {
		opt(s)
	}

	// Enable CORS
	app.Use(cors.New())

	// Register routes
	s.setupRoutes(world, messages, components)

	return s, nil
}

// Serve serves the application, blocking the calling thread.
// Call this in a new go routine to prevent blocking.
func (s *Server) Serve(ctx context.Context) error {
	serverErr := make(chan error, 1)

	// Starts the server in a new goroutine
	go func() {
		log.Info().Msgf("Starting HTTP server at port %s", s.config.port)
		if err := s.app.Listen(":" + s.config.port); err != nil {
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

// @title			Cardinal
// @description	Backend server for World Engine
// @version		0.0.1
// @schemes		http ws
// @BasePath		/
// @consumes		application/json
// @produces		application/json
func (s *Server) setupRoutes(
	world servertypes.ProviderWorld,
	messages []types.Message,
	components []types.ComponentMetadata,
) {
	// /tx/:group/:txType
	// maps group -> txType -> tx
	msgIndex := make(map[string]map[string]types.Message)

	// Create tx index
	for _, msg := range messages {
		// Initialize inner map if it doesn't exist
		if _, ok := msgIndex[msg.Group()]; !ok {
			msgIndex[msg.Group()] = make(map[string]types.Message)
		}
		msgIndex[msg.Group()][msg.Name()] = msg
	}

	// Route: /swagger/
	if !s.config.isSwaggerDisabled {
		s.app.Get("/swagger/*", swagger.HandlerDefault)
	}

	// Route: /events/
	s.app.Use("/events", handler.WebSocketUpgrader)
	s.app.Get("/events", handler.WebSocketEvents())

	// Route: /world
	s.app.Get("/world", handler.GetWorld(world, components, messages, world.Namespace()))

	// Route: /...
	s.app.Get("/health", handler.GetHealth())

	// Route: /query/...
	query := s.app.Group("/query")
	query.Post("/receipts/list", handler.GetReceipts(world))
	query.Post("/:group/:name", handler.PostQuery(world))

	// Route: /tx/...
	tx := s.app.Group("/tx")
	tx.Post("/:group/:name", handler.PostTransaction(world, msgIndex, s.config.isSignatureVerificationDisabled))

	// Route: /cql
	s.app.Post("/cql", handler.PostCQL(world))

	// Route: /debug/state
	s.app.Post("/debug/state", handler.GetState(world))
}
