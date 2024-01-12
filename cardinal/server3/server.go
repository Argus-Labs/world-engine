package server3

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
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

	disableSignatureVerification bool
	withCORS                     bool

	running       atomic.Bool
	shutdownMutex sync.Mutex
}

func New(eng *ecs.Engine, opts ...Option) *Server {
	s := &Server{
		eng:  eng,
		app:  fiber.New(),
		port: defaultPort,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Server) registerHandlers() error {

}

func (s *Server) getTransactionFromBody(bz []byte) (*sign.Transaction, error) {
	tx := new(sign.Transaction)
	err := json.Unmarshal(bz, tx)
	return tx, err
}

func (s *Server) registerMessageHandlers() error {
	msgs, err := s.eng.ListMessages()
	if err != nil {
		return err
	}
	msgNameToMsg := make(map[string]message.Message)
	for _, msg := range msgs {
		msgNameToMsg[msg.Name()] = msg
	}

	s.app.Post("/tx/game/:{tx_type}", func(ctx *fiber.Ctx) error {
		body := ctx.Body()
		if len(body) == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "request body was empty")
		}
		tx, err := s.getTransactionFromBody(body)
		if err != nil {
			return err
		}

	})
}

var (
	ErrNoPersonaTag               = errors.New("persona tag is required")
	ErrWrongNamespace             = errors.New("incorrect namespace")
	ErrSystemTransactionRequired  = errors.New("system transaction required")
	ErrSystemTransactionForbidden = errors.New("system transaction forbidden")
)

func (s *Server) validateTransaction(tx *sign.Transaction, systemTx bool) error {
	if s.disableSignatureVerification {
		return nil
	}
	if tx.PersonaTag == "" {
		return ErrNoPersonaTag
	}
	if tx.Namespace != s.eng.Namespace().String() {
		return fmt.Errorf("expected %q got %q: %w", s.eng.Namespace().String(), tx.Namespace, ErrWrongNamespace)
	}

	if systemTx && !tx.IsSystemTransaction() {
		return ErrSystemTransactionRequired
	}
	if !systemTx && tx.IsSystemTransaction() {
		return ErrSystemTransactionForbidden
	}

	if !systemTx {

	}
}
