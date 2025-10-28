package main

import (
	"github.com/argus-labs/world-engine/pkg/template/multi-shard/shards/chat/system"

	"github.com/argus-labs/world-engine/pkg/cardinal"
)

func main() {
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:       20,
		EpochFrequency: 200,
	})
	if err != nil {
		panic(err.Error())
	}

	cardinal.RegisterSystem(world, system.UserChatSystem)

	world.StartGame()
}
