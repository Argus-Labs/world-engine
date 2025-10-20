package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/demo-game/system"
)

func main() {
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:       20,
		EpochFrequency: 200,
	})
	if err != nil {
		panic(err.Error())
	}

	cardinal.RegisterSystem(world, system.PlayerSetUpdater, cardinal.WithHook(cardinal.PreUpdate))
	cardinal.RegisterSystem(world, system.PlayerSpawnSystem)
	cardinal.RegisterSystem(world, system.MovePlayerSystem)
	cardinal.RegisterSystem(world, system.PlayerLeaveSystem)
	cardinal.RegisterSystem(world, system.OnlineStatusUpdater)

	world.StartGame()
}
