package lobby_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/lobby"
	"github.com/stretchr/testify/require"
)

func TestDST(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		cardinal.RegisterPlugin(w, lobby.NewPlugin(lobby.Config{}))
	})
}

func TestE2E(t *testing.T) {
	cardinal.RunE2E(t, func() *cardinal.World {
		debug := false

		world, err := cardinal.NewWorld(cardinal.WorldOptions{
			Region:              "local",
			Organization:        "organization",
			Project:             "project",
			ShardID:             "lobby",
			TickRate:            1,
			SnapshotRate:        50,
			SnapshotStorageType: snapshot.StorageTypeJetStream,
			Debug:               &debug,
		})
		require.NoError(t, err)

		cardinal.RegisterPlugin(world, lobby.NewPlugin(lobby.Config{}))

		return world
	})
}
