package multishard_test

import (
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal"
	otherworld "github.com/argus-labs/world-engine/pkg/template/multi-shard/pkg/other_world"
	chatcommand "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/command"
	chatcomponent "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/component"
	chatevent "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/event"
	chatsystem "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/system"
	gamecommand "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/command"
	gamecomponent "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/component"
	gameevent "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/event"
	gamesystem "github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/game/system"
)

func TestDSTGame(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		w.RegisterComponent[gamecomponent.PlayerTag]()
		w.RegisterComponent[gamecomponent.Position]()
		w.RegisterComponent[gamecomponent.OnlineStatus]()
		w.RegisterCommand[gamecommand.PlayerSpawn]()
		w.RegisterCommand[gamecommand.MovePlayer]()
		w.RegisterCommand[gamecommand.PlayerLeave]()
		w.RegisterEvent[gameevent.PlayerSpawn]()
		w.RegisterEvent[gameevent.PlayerMovement]()
		w.RegisterEvent[gameevent.PlayerDeparture]()
		w.RegisterSystem(&gamesystem.PlayerSpawnSystem{ChatWorld: otherworld.Chat()})
		w.RegisterSystem(&gamesystem.MovePlayerSystem{})
		w.RegisterSystem(&gamesystem.PlayerLeaveSystem{})
		w.RegisterSystem(&gamesystem.OnlineStatusUpdater{})
	}, nil)
}

func TestDSTChat(t *testing.T) {
	cardinal.RunDST(t, func(w *cardinal.World) {
		w.RegisterComponent[chatcomponent.UserTag]()
		w.RegisterComponent[chatcomponent.Chat]()
		w.RegisterCommand[chatcommand.UserChat]()
		w.RegisterEvent[chatevent.UserChat]()
		w.RegisterSystem(&chatsystem.UserChatSystem{})
	}, nil)
}
