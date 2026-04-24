package lobby_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/snapshot"
	"github.com/argus-labs/world-engine/pkg/plugin/lobby"
	"github.com/stretchr/testify/require"
)

// testOrchestratorState is the system state for a minimal orchestrator
// that immediately assigns every awaiting-allocation lobby to a fixed
// game shard. This ensures DST/E2E fuzzing exercises the full session
// lifecycle (awaiting_allocation → in_session → idle) instead of
// parking at awaiting_allocation forever.
type testOrchestratorState struct {
	cardinal.BaseSystemState
	Lobbies cardinal.Contains[struct {
		Lobby cardinal.Ref[lobby.Component]
	}]
}

func testOrchestratorSystem(state *testOrchestratorState) {
	self := cardinal.OtherWorld{
		Region:       "local",
		Organization: "organization",
		Project:      "project",
		ShardID:      "lobby",
	}
	for _, refs := range state.Lobbies.Iter() {
		lob := refs.Lobby.Get()
		if lob.Session.State != lobby.SessionStateAwaitingAllocation {
			continue
		}
		self.SendCommand(&state.BaseSystemState, lobby.AssignShardCommand{
			LobbyID:   lob.ID,
			RequestID: lob.Session.PendingRequestID,
			GameWorld: cardinal.OtherWorld{
				Region:       "local",
				Organization: "organization",
				Project:      "project",
				ShardID:      "game-test-1",
			},
		})
	}
}

func TestDST(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		cardinal.RegisterPlugin(w, lobby.NewPlugin(lobby.Config{}))
		cardinal.RegisterSystem(w, testOrchestratorSystem)
	}, nil)
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
		cardinal.RegisterSystem(world, testOrchestratorSystem)

		return world
	})
}
