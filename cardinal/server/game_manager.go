package server

import (
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type GameManager struct {
	handler *Handler
	world   *ecs.World
}

func NewGameManager(world *ecs.World, handler *Handler) GameManager {

	manager := GameManager{
		handler: handler,
		world:   world,
	}

	//handle shutdown via a signal
	signalChannel := make(chan os.Signal, 1)
	go func() {
		signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
		for sig := range signalChannel {
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				err := manager.Shutdown()
				if err != nil {
					log.Err(err).Msgf("There was an error during shutdown.")
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
		return errors.New("game manager has no server, can't shutdown")
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
