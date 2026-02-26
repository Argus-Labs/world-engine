package multishard_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	chatsystem "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/system"
	gamesystem "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/system"
)

func TestDSTGame(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		cardinal.RegisterSystem(w, gamesystem.PlayerSetUpdater, cardinal.WithHook(cardinal.PreUpdate))
		cardinal.RegisterSystem(w, gamesystem.PlayerSpawnSystem)
		cardinal.RegisterSystem(w, gamesystem.MovePlayerSystem)
		cardinal.RegisterSystem(w, gamesystem.PlayerLeaveSystem)
		cardinal.RegisterSystem(w, gamesystem.OnlineStatusUpdater)
	})
}

func TestDSTChat(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		cardinal.RegisterSystem(w, chatsystem.UserChatSystem)
	})
}
