package server

import (
	_ "embed"
	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/server/handler"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"sync/atomic"
)

const (
	defaultPort = "4040"
)

var (
	//go:embed swagger.yml
	swaggerData []byte
)

type Server struct {
	eng *ecs.Engine
	app *fiber.App

	port string

	txPrefix      string
	queryPrefix   string
	txWildCard    string
	queryWildCard string

	disableSignatureVerification bool
	withCORS                     bool
	disableSwagger               bool

	running atomic.Bool
}

// New returns an HTTP server with handlers for all QueryTypes and MessageTypes.
func New(eng *ecs.Engine, opts ...Option) (*Server, error) {
	s := &Server{
		eng:           eng,
		app:           fiber.New(),
		txPrefix:      "/tx/game/",
		txWildCard:    "txType",
		queryPrefix:   "/query/game/",
		queryWildCard: "queryType",
		port:          defaultPort,
	}
	for _, opt := range opts {
		opt(s)
	}

	if !s.disableSwagger {
		if err := s.setupSwagger(); err != nil {
			return nil, err
		}
	}

	s.setupRoutes()

	return s, nil
}

// Port returns the port the server will run on.
func (s *Server) Port() string {
	return s.port
}

// Serve serves the application, blocking the calling thread.
// Call this in a new go routine to prevent blocking.
func (s *Server) Serve() error {
	hostname, err := os.Hostname()
	if err != nil {
		return eris.Wrap(err, "error getting hostname")
	}
	log.Info().Msgf("serving at %s:%s", hostname, s.port)
	s.running.Store(true)
	err = s.app.Listen(":" + s.port)
	if err != nil {
		return eris.Wrap(err, "error starting Fiber app")
	}
	s.running.Store(false)
	return nil
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
func (s *Server) Shutdown() error {
	return s.app.Shutdown()
}

func (s *Server) setupSwagger() error {
	file, err := os.CreateTemp("", "")
	if err != nil {
		return eris.Wrap(err, "failed to crate temp file for swagger")
	}
	_, err = file.Write(swaggerData)
	if err != nil {
		return eris.Wrap(err, "failed to write swagger data to file")
	}
	// Setup swagger docs at /docs
	cfg := swagger.Config{
		FilePath: file.Name(),
		Title:    "World Engine API Docs",
	}
	s.app.Use(swagger.New(cfg))

	if err := os.Remove(file.Name()); err != nil {
		return eris.Wrap(err, "failed to remove swagger temp file")
	}
	return nil
}

func (s *Server) setupRoutes() {
	// split messages based on whether they supplied their own custom path.
	msgSlice := s.eng.ListMessages()
	messages := make(map[string]message.Message)
	customPathMessages := make(map[string]message.Message)
	for _, msg := range msgSlice {
		if msg.Path() == "" {
			messages[msg.Name()] = msg
		} else {
			customPathMessages[msg.Path()] = msg
		}
	}

	// split queries based on whether they supplied their own custom path.
	querySlice := s.eng.ListQueries()
	queries := make(map[string]ecs.Query)
	customPathQuery := make(map[string]ecs.Query)
	for _, q := range querySlice {
		if q.Path() == "" {
			queries[q.Name()] = q
		} else {
			customPathQuery[q.Path()] = q
		}
	}

	s.app.Get("/health", handler.GetHealth(s.eng))
	s.app.Get("/query/http/endpoints", handler.GetEndpoints(msgSlice, querySlice, s.txPrefix, s.queryPrefix))
	s.app.Post("/query/game/:queryType", handler.PostQuery(queries, s.eng, s.queryWildCard))
	s.app.Post("/tx/game/:txType", handler.PostTransaction(messages, s.eng, s.disableSignatureVerification, s.txWildCard))

	for _, query := range customPathQuery {
		s.app.Post(query.Path(), handler.PostCustomPathQuery(query, s.eng))
	}
	for _, msg := range customPathMessages {
		s.app.Post(msg.Path(), handler.PostCustomPathTransaction(msg, s.eng, s.disableSignatureVerification))
	}
}
