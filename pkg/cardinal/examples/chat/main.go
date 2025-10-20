package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
	"github.com/argus-labs/world-engine/pkg/cardinal/examples/chat/system"
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
