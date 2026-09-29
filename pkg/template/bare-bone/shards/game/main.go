package main

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
)

func main() {
	w, err := cardinal.NewWorld(cardinal.WorldOptions{
		TickRate:     1,
		SnapshotRate: 50,
	})
	if err != nil {
		panic(err.Error())
	}

	// Register components
	// w.RegisterComponent[component.ExampleComponent]()

	// Register systems
	// w.RegisterSystem(&system.ExampleSystem{})

	w.StartGame()
}
