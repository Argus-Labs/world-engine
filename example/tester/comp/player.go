package comp

import "pkg.world.dev/world-engine/cardinal/ecs"

type Player struct {
	Name string
}

var PlayerComponent = ecs.NewComponentType[Player]()
