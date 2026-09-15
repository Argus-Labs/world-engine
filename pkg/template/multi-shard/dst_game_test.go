package multishard_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	chatsystem "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/system"
	gamesystem "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/system"
)

func TestDSTGame(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		w.RegisterSystem(gamesystem.PlayerSetUpdater, cardinal.WithHook(cardinal.PreUpdate))
		w.RegisterSystem(gamesystem.PlayerSpawnSystem)
		w.RegisterSystem(gamesystem.MovePlayerSystem)
		w.RegisterSystem(gamesystem.PlayerLeaveSystem)
		w.RegisterSystem(gamesystem.OnlineStatusUpdater)
	}, nil)
}

func TestDSTChat(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		w.RegisterSystem(chatsystem.UserChatSystem)
	}, nil)
}
