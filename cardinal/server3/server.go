package server3

import (
	"errors"
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"sync"
	"sync/atomic"
)

const (
	defaultPort = "4040"
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
	err := s.registerHandlers()
	return s, err
}

func (s *Server) registerHandlers() error {
	return errors.Join(
		s.registerTransactionHandler(s.txPrefix+":{tx_type}"),
		s.registerQueryHandler(s.queryPrefix+":{query_type}"),
		s.registerListEndpointsEndpoint("/query/http/endpoints"),
	)
}
