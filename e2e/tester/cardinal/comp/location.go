package comp

import "pkg.world.dev/world-engine/cardinal/ecs"

type Location struct {
	X, Y int64
}

var LocationComponent = ecs.NewComponentType[Location]()
