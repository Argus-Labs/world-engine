package main_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system"
	"github.com/stretchr/testify/require"
)

func TestDST(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		registerSystems(w)
	})
}

func TestE2E(t *testing.T) {
	cardinal.RunE2E(t, func() *cardinal.World {
		debug := false

		// Keep world setup aligned with shards/game/main.go.
		world, err := cardinal.NewWorld(cardinal.WorldOptions{
			Region:              "local",
			Organization:        "organization",
			Project:             "project",
			ShardID:             "game",
			TickRate:            1,
			SnapshotRate:        50,
			SnapshotStorageType: snapshot.StorageTypeJetStream,
			Debug:               &debug,
		})
		require.NoError(t, err)

		registerSystems(world)

		return world
	})
}

func registerSystems(w *cardinal.World) {
	cardinal.RegisterSystem(w, system.PlayerSpawnerSystem, cardinal.WithHook(cardinal.Init))
	cardinal.RegisterSystem(w, system.CreatePlayerSystem)
	cardinal.RegisterSystem(w, system.RegenSystem)
	cardinal.RegisterSystem(w, system.AttackPlayerSystem)
	cardinal.RegisterSystem(w, system.GraveyardSystem)
	cardinal.RegisterSystem(w, system.CallExternalSystem)
}
