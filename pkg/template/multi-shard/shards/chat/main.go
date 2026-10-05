package main

import (
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/command"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/component"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/event"
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/system"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

func main() {
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:     20,
		SnapshotRate: 50,
	})
	if err != nil {
		panic(err.Error())
	}

	w.RegisterComponent[component.UserTag]()
	w.RegisterComponent[component.Chat]()

	w.RegisterCommand[command.UserChat]()

	w.RegisterEvent[event.UserChat]()

	w.RegisterSystem(&system.UserChatSystem{})

	w.StartGame()
}
