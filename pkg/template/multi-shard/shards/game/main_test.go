package main_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/system"
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
			TickRate:            20,
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
	w.RegisterComponent[component.Position]()
	w.RegisterComponent[component.OnlineStatus]()

	w.RegisterSystem(system.PlayerSetUpdater, cardinal.WithHook(cardinal.PreUpdate))
	w.RegisterSystem(system.PlayerSpawnSystem)
	w.RegisterSystem(system.MovePlayerSystem)
	w.RegisterSystem(system.PlayerLeaveSystem)
	w.RegisterSystem(system.OnlineStatusUpdater)
}
