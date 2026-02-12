package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

func main() {
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:            1,
		SnapshotRate:        50,
		SnapshotStorageType: snapshot.StorageTypeJetStream,
	})
	if err != nil {
		panic(err.Error())
	}

	cardinal.RegisterSystem(world, system.PlayerSpawnerSystem, cardinal.WithHook(cardinal.Init))

	cardinal.RegisterSystem(world, system.CreatePlayerSystem)
	cardinal.RegisterSystem(world, system.RegenSystem)
	cardinal.RegisterSystem(world, system.AttackPlayerSystem)
	cardinal.RegisterSystem(world, system.GraveyardSystem)
	cardinal.RegisterSystem(world, system.CallExternalSystem)

	world.StartGame()
}
