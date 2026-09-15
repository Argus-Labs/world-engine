package main

import (
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/system"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

func main() {
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:     20,
		SnapshotRate: 50,
	})
	if err != nil {
		panic(err.Error())
	}

	w.RegisterSystem(system.PlayerSetUpdater, cardinal.WithHook(cardinal.PreUpdate))
	w.RegisterSystem(system.PlayerSpawnSystem)
	w.RegisterSystem(system.MovePlayerSystem)
	w.RegisterSystem(system.PlayerLeaveSystem)
	w.RegisterSystem(system.OnlineStatusUpdater)

	w.StartGame()
}
