package server

import (
	_ "embed"
	"github.com/gofiber/contrib/websocket"
	"os"
	"sync/atomic"

	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/server/handler"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

const (
	DefaultPort = "4040"
)

var (
	//go:embed swagger.yml
	swaggerData []byte
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
func New(engine *ecs.Engine, wsEventHandler func(conn *websocket.Conn), opts ...Option) (*Server, error) {
	app := fiber.New()
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
	setupRoutes(app, engine, wsEventHandler, s.config)

	if !s.config.isSwaggerDisabled {
		if err := setupSwagger(app); err != nil {
			return nil, err
		}
	}

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

func setupSwagger(app *fiber.App) error {
	file, err := os.CreateTemp("", "")
	if err != nil {
		return eris.Wrap(err, "failed to crate temp file for swagger")
	}
	defer func() {
		if err := os.Remove(file.Name()); err != nil {
			log.Error().Err(err).Msgf("failed to delete temporary swagger file %q", file.Name())
		}
	}()
	_, err = file.Write(swaggerData)
	if err != nil {
		return eris.Wrap(err, "failed to write swagger data to file")
	}

	// Register swagger docs at /docs
	cfg := swagger.Config{
		FilePath: file.Name(),
		Title:    "World Engine API Docs",
	}
	app.Use(swagger.New(cfg))

	return nil
}

func setupRoutes(app *fiber.App, engine *ecs.Engine, wsEventHandler func(conn *websocket.Conn), cfg config) {
	// TODO(scott): we should refactor this such that we only dependency inject these maps
	//  instead of having to dependency inject the entire engine.
	// /query/:group/:queryType
	// maps group -> queryType -> query
	queries := make(map[string]map[string]ecs.Query)

	// /tx/:group/:txType
	// maps group -> txType -> tx
	msgs := make(map[string]map[string]message.Message)

	// Create query index
	for _, query := range engine.ListQueries() {
		// Initialize inner map if it doesn't exist
		if _, ok := queries[query.Group()]; !ok {
			queries[query.Group()] = make(map[string]ecs.Query)
		}
		queries[query.Group()][query.Name()] = query
	}

	// Create tx index
	for _, msg := range engine.ListMessages() {
		// Initialize inner map if it doesn't exist
		if _, ok := msgs[msg.Group()]; !ok {
			msgs[msg.Group()] = make(map[string]message.Message)
		}
		msgs[msg.Group()][msg.Name()] = msg
	}

	// Route: /events/
	app.Use("/events", handler.WebSocketUpgrader)
	app.Get("/events", handler.WebSocketEvents(wsEventHandler))

	// Route: /...
	app.Get("/health", handler.GetHealth(engine))
	// TODO(scott): this should be moved outside of /query, but nakama is currrently depending on it
	//  so we should do this on a separate PR.
	app.Get("/query/http/endpoints", handler.GetEndpoints(msgs, queries))

	// Route: /query/...
	query := app.Group("/query")
	query.Post("/:group/:name", handler.PostQuery(queries, engine))

	// Route: /tx/...
	tx := app.Group("/tx")
	tx.Post("/:group/:name", handler.PostTransaction(msgs, engine, cfg.isSignatureVerificationDisabled))
}
