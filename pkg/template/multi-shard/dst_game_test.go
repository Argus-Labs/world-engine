package multishard_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	chatcomponent "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/component"
	chatsystem "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/system"
	gamecomponent "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	gamesystem "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/system"
)

func TestDSTGame(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		w.RegisterComponent[gamecomponent.PlayerTag]()
		w.RegisterComponent[gamecomponent.Position]()
		w.RegisterComponent[gamecomponent.OnlineStatus]()
		w.RegisterSystem(gamesystem.PlayerSetUpdater, cardinal.WithHook(cardinal.PreUpdate))
		w.RegisterSystem(gamesystem.PlayerSpawnSystem)
		w.RegisterSystem(gamesystem.MovePlayerSystem)
		w.RegisterSystem(gamesystem.PlayerLeaveSystem)
		w.RegisterSystem(gamesystem.OnlineStatusUpdater)
	}, nil)
}

func TestDSTChat(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		w.RegisterComponent[chatcomponent.UserTag]()
		w.RegisterComponent[chatcomponent.Chat]()
		w.RegisterSystem(chatsystem.UserChatSystem)
	}, nil)
}
