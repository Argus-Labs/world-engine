package main

import (
	otherworld "github.com/argus-labs/world-engine/pkg/template/multi-shard/pkg/other_world"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"
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

	w.RegisterComponent[component.PlayerTag]()
	w.RegisterComponent[component.Position]()
	w.RegisterComponent[component.OnlineStatus]()

	w.RegisterCommand[command.PlayerSpawn]()
	w.RegisterCommand[command.MovePlayer]()
	w.RegisterCommand[command.PlayerLeave]()

	w.RegisterEvent[event.PlayerSpawn]()
	w.RegisterEvent[event.PlayerMovement]()
	w.RegisterEvent[event.PlayerDeparture]()

	w.RegisterSystem(&system.PlayerSpawnSystem{ChatWorld: otherworld.Chat()})
	w.RegisterSystem(&system.MovePlayerSystem{})
	w.RegisterSystem(&system.PlayerLeaveSystem{})
	w.RegisterSystem(&system.OnlineStatusUpdater{})

	w.StartGame()
}
