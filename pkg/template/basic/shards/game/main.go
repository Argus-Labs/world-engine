package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	otherworld "github.com/argus-labs/world-engine/pkg/template/basic/pkg/other_worlds"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/event"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system"
	systemevent "github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system_event"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

func main() {
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:            1,
		SnapshotRate:        50,
		SnapshotStorageType: snapshot.StorageTypeJetStream,
	})
	if err != nil {
		panic(err.Error())
	}

	w.RegisterComponent[component.PlayerTag]()
	w.RegisterComponent[component.Health]()
	w.RegisterComponent[component.Gravestone]()

	w.RegisterCommand[system.CreatePlayerCommand]()
	w.RegisterCommand[system.AttackPlayerCommand]()
	w.RegisterCommand[system.CallExternalCommand]()

	w.RegisterEvent[event.NewPlayer]()
	w.RegisterEvent[event.PlayerDeath]()

	w.RegisterSystemEvent[systemevent.PlayerDeath]()

	w.RegisterSystem(&system.PlayerSpawnerSystem{}, cardinal.WithHook(cardinal.Init))

	w.RegisterSystem(&system.CreatePlayerSystem{})
	w.RegisterSystem(&system.RegenSystem{})
	w.RegisterSystem(&system.AttackPlayerSystem{})
	w.RegisterSystem(&system.GraveyardSystem{})
	w.RegisterSystem(&system.CallExternalSystem{MatchmakingWorld: otherworld.Matchmaking()})

	w.StartGame()
}
