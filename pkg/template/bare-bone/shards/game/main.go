package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
)

func main() {
	world, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:       1,
		EpochFrequency: 10,
	})
	if err != nil {
		panic(err.Error())
	}

	// Register systems
	// cardinal.RegisterSystem(world, system.ExampleSystem)

	world.StartGame()
}
