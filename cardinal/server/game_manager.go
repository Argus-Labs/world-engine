package server

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type GameManager struct {
	handler *Handler
	world   *ecs.World
}

type GameManagerOptions = func(g *GameManager)

func WithGameManagerPrettyPrint(_ *GameManager) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

func NewGameManager(world *ecs.World, handler *Handler, options ...GameManagerOptions) GameManager {
	manager := GameManager{
		handler: handler,
		world:   world,
	}
	for _, option := range options {
		option(&manager)
	}

	// handle shutdown via a signal
	signalChannel := make(chan os.Signal, 1)
	go func() {
		signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
		for sig := range signalChannel {
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				err := manager.Shutdown()
				if err != nil {
					log.Err(err).Msgf(eris.ToString(err, true))
				}
				return
			}
		}
	}()
	return manager
}

func (s *GameManager) Shutdown() error {
	log.Info().Msg("Shutting down server.")
	if s.handler == nil {
		return eris.New("game manager has no server, can't shutdown")
	}
	err := s.handler.Shutdown()
	if err != nil {
		return err
	}
	log.Info().Msg("Server successfully shutdown.")
	log.Info().Msg("Shutting down game loop.")
	s.world.Shutdown()
	err = s.world.StoreManager().Close()
	if err != nil {
		return err
	}
	log.Info().Msg("Successfully shutdown server and game loop.")
	return nil
}
