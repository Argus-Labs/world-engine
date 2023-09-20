package server

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"pkg.world.dev/world-engine/cardinal/ecs"
)

type ShutDownManager struct {
	handler *Handler
	world   *ecs.World
}

func NewShutdownManager(world *ecs.World, handler *Handler) ShutDownManager {

	manager := ShutDownManager{
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
					fmt.Println("There was an error during shutdown %w", err)
					return
				}
			}
		}
	}()
	return manager
}

func (s *ShutDownManager) Shutdown() error {
	err := s.handler.Shutdown()
	if err != nil {
		return err
	}
	s.world.EndGameLoop()
	fmt.Println("Successfully shutdown server and game loop.")
	return nil
}
