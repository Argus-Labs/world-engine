package server3

import (
	_ "embed"
	"errors"
	"github.com/gofiber/contrib/swagger"
	"github.com/gofiber/fiber/v2"
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

	txPrefix    string
	queryPrefix string

	disableSignatureVerification bool
	withCORS                     bool

	running       atomic.Bool
	shutdownMutex sync.Mutex
}

func New(eng *ecs.Engine, opts ...Option) (*Server, error) {
	s := &Server{
		eng:         eng,
		app:         fiber.New(),
		txPrefix:    "/tx/game/",
		queryPrefix: "/query/game/",
		port:        defaultPort,
	}
	for _, opt := range opts {
		opt(s)
	}

	s.setupSwagger()

	err := s.registerHandlers()
	return s, err
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
		s.registerTransactionHandler(s.txPrefix+":{tx_type}"),
		s.registerQueryHandler(s.queryPrefix+":{query_type}"),
		s.registerListEndpointsEndpoint("/query/http/endpoints"),
	)
}
