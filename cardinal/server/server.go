package server

import (
	"encoding/json"
	"os"

	"github.com/gofiber/contrib/socketio"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/swagger"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/server/handler"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"

	_ "pkg.world.dev/world-engine/cardinal/server/docs" // for swagger.
)

const (
	DefaultPort = "4040"
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
	provider servertypes.Provider, wCtx engine.Context, components []types.ComponentMetadata,
	messages []types.Message, queries []engine.Query, opts ...Option,
) (*Server, error) {
	app := fiber.New(fiber.Config{
		Network: "tcp", // Enable server listening on both ipv4 & ipv6 (default: ipv4 only)
	})

	s := &Server{
		app: app,
		config: config{
			port:                            DefaultPort,
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
	s.setupRoutes(provider, wCtx, messages, queries, components)

	return s, nil
}

// Serve serves the application, blocking the calling thread.
// Call this in a new go routine to prevent blocking.
func (s *Server) Serve() error {
	hostname, err := os.Hostname()
	if err != nil {
		return eris.Wrap(err, "error getting hostname")
	}

	// Start server
	log.Info().Msgf("serving at %s:%s", hostname, s.config.port)
	err = s.app.Listen(":" + s.config.port)
	if err != nil {
		return eris.Wrap(err, "error starting Fiber app")
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
func (s *Server) Shutdown() error {
	log.Info().Msg("Shutting down server")

	// Close websocket connections
	socketio.Broadcast([]byte(""), socketio.CloseMessage)
	socketio.Fire(socketio.EventClose, nil)

	// Gracefully shutdown Fiber server
	if err := s.app.Shutdown(); err != nil {
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
	provider servertypes.Provider, wCtx engine.Context, messages []types.Message,
	queries []engine.Query, components []types.ComponentMetadata,
) {
	// TODO(scott): we should refactor this such that we only dependency inject these maps
	//  instead of having to dependency inject the entire engine.
	// /query/:group/:queryType
	// maps group -> queryType -> query
	queryIndex := make(map[string]map[string]engine.Query)

	// /tx/:group/:txType
	// maps group -> txType -> tx
	msgIndex := make(map[string]map[string]types.Message)

	// Create query index
	for _, query := range queries {
		// Initialize inner map if it doesn't exist
		if _, ok := queryIndex[query.Group()]; !ok {
			queryIndex[query.Group()] = make(map[string]engine.Query)
		}
		queryIndex[query.Group()][query.Name()] = query
	}

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
	s.app.Get("/world", handler.GetWorld(components, messages, queries, wCtx.Namespace()))

	// Route: /...
	s.app.Get("/health", handler.GetHealth())
	// TODO(scott): this should be moved outside of /query, but nakama is currrently depending on it
	//  so we should do this on a separate PR.
	s.app.Get("/query/http/endpoints", handler.GetEndpoints(msgIndex, queryIndex))

	// Route: /query/...
	query := s.app.Group("/query")
	query.Post("/:group/:name", handler.PostQuery(queryIndex, wCtx))

	// Route: /tx/...
	tx := s.app.Group("/tx")
	tx.Post("/:group/:name", handler.PostTransaction(provider, msgIndex, s.config.isSignatureVerificationDisabled))

	// Route: /cql
	s.app.Post("/cql", handler.PostCQL(provider))
}
