package main_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/basic/shards/game/system"
	"github.com/stretchr/testify/require"
)

func TestDST(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		registerSystems(w)
	}, nil)
}

func TestE2E(t *testing.T) {
	cardinal.RunE2E(t, func() *cardinal.World {
		debug := false

		// Keep world setup aligned with shards/game/main.go.
		w, err := cardinal.NewWorld(cardinal.WorldOptions{
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

		registerSystems(w)

		return w
	})
}

func registerSystems(w *cardinal.World) {
	w.RegisterComponent[component.PlayerTag]()
	w.RegisterComponent[component.Health]()
	w.RegisterComponent[component.Gravestone]()

	w.RegisterSystem(system.PlayerSpawnerSystem, cardinal.WithHook(cardinal.Init))
	w.RegisterSystem(system.CreatePlayerSystem)
	w.RegisterSystem(system.RegenSystem)
	w.RegisterSystem(system.AttackPlayerSystem)
	w.RegisterSystem(system.GraveyardSystem)
	w.RegisterSystem(system.CallExternalSystem)
}
