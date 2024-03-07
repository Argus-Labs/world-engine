package server

import (
	"os"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"sync/atomic"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/swagger"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/server/handler"

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
	app       *fiber.App
	config    config
	isRunning atomic.Bool
}

// New returns an HTTP server with handlers for all QueryTypes and MessageTypes.
func New(
	wCtx engine.Context, components []types.ComponentMetadata, messages []types.Message,
	queries []engine.Query, wsEventHandler func(conn *websocket.Conn),
	opts ...Option,
) (*Server, error) {
	app := fiber.New(fiber.Config{
		// Enable server listening on both ipv4 & ipv6 (default: ipv4 only)
		Network: "tcp",
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
	setupRoutes(app, wCtx, messages, queries, wsEventHandler, s.config, components)

	return s, nil
}

// Port returns the port the server listens to.
func (s *Server) Port() string {
	return s.config.port
}

// Serve serves the application, blocking the calling thread.
// Call this in a new go routine to prevent blocking.
func (s *Server) Serve() error {
	hostname, err := os.Hostname()
	if err != nil {
		return eris.Wrap(err, "error getting hostname")
	}
	log.Info().Msgf("serving at %s:%s", hostname, s.config.port)
	s.isRunning.Store(true)
	err = s.app.Listen(":" + s.config.port)
	if err != nil {
		return eris.Wrap(err, "error starting Fiber app")
	}
	s.isRunning.Store(false)
	return nil
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}

// @title			Cardinal
// @description	Backend server for World Engine
// @version		0.0.1
// @schemes		http ws
// @BasePath		/
// @consumes		application/json
// @produces		application/json
func setupRoutes(
	app *fiber.App, wCtx engine.Context, messages []types.Message, queries []engine.Query,
	wsEventHandler func(conn *websocket.Conn),
	cfg config, components []types.ComponentMetadata,
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
	if !cfg.isSwaggerDisabled {
		app.Get("/swagger/*", swagger.HandlerDefault)
	}

	// Route: /events/
	app.Use("/events", handler.WebSocketUpgrader)
	app.Get("/events", handler.WebSocketEvents(wsEventHandler))

	// Route: /world
	app.Get("/world", handler.GetWorld(components, messages, queries))

	// Route: /...
	app.Get("/health", handler.GetHealth())
	// TODO(scott): this should be moved outside of /query, but nakama is currrently depending on it
	//  so we should do this on a separate PR.
	app.Get("/query/http/endpoints", handler.GetEndpoints(msgIndex, queryIndex))

	// Route: /query/...
	query := app.Group("/query")
	query.Post("/:group/:name", handler.PostQuery(queryIndex, wCtx))

	// Route: /tx/...
	tx := app.Group("/tx")
	tx.Post("/:group/:name", handler.PostTransaction(msgIndex, wCtx, cfg.isSignatureVerificationDisabled))
}
