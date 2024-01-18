package server3

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"os"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"sync"
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

	running       atomic.Bool
	shutdownMutex sync.Mutex
}

func New(eng *ecs.Engine, opts ...Option) (*Server, error) {
	s := &Server{
		eng:           eng,
		app:           fiber.New(),
		txPrefix:      "/tx/game/",
		txWildCard:    "{txType}",
		queryPrefix:   "/query/game/",
		queryWildCard: "{queryType}",
		port:          defaultPort,
	}
	for _, opt := range opts {
		opt(s)
	}

	if !s.disableSwagger {
		s.setupSwagger()
	}

	err := s.registerHandlers()
	// Print the router stack in JSON format
	data, _ := json.MarshalIndent(s.app.Stack(), "", "  ")
	fmt.Println(string(data))
	return s, err
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

func (s *Server) Shutdown() {
	err := s.app.Shutdown()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to shutdown server")
	}
}

func (s *Server) setupSwagger() {
	file, err := os.CreateTemp("", "")
	if err != nil {
		panic("could not create temp file for swaggerFile")
	}
	_, err = file.Write(swaggerData)
	if err != nil {
		panic("could not write swaggerFile to temp file")
	}
	// Setup swagger docs at /docs
	cfg := swagger.Config{
		FilePath: file.Name(),
		Title:    "World Engine API Docs",
	}
	s.app.Use(swagger.New(cfg))
}

func (s *Server) registerHandlers() error {

	return errors.Join(
		s.registerTransactionHandler(fmt.Sprintf("%s:%s", s.txPrefix, s.txWildCard)),
		s.registerQueryHandler(fmt.Sprintf("%s:%s", s.queryPrefix, s.queryWildCard)),
		s.registerListEndpointsEndpoint("/query/http/endpoints"),
		s.registerHealthEndpoint("/health"),
	)
}
