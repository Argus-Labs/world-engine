package server1

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
	world   *ecs.Engine
}

func (g *GameManager) IsRunning() bool {
	return g.handler.running.Load() && g.world.IsGameLoopRunning()
}

type GameManagerOptions func(g *GameManager)

func WithGameManagerPrettyPrint(_ *GameManager) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
}

func NewGameManager(world *ecs.Engine, handler *Handler, options ...GameManagerOptions) GameManager {
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

func (g *GameManager) Shutdown() error {
	if g.handler == nil {
		return eris.New("game manager has no server, can't shutdown")
	}
	err := g.handler.Shutdown()
	if err != nil {
		return err
	}
	g.world.Shutdown()
	err = g.world.StoreManager().Close()
	if err != nil {
		return err
	}
	if !g.IsRunning() {
		log.Info().Msg("Successfully shutdown server and game loop.")
	}
	return nil
}
