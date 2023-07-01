package component

import "github.com/argus-labs/world-engine/cardinal/ecs"

type PositionComponent struct {
	X, Y int
}

var Position = ecs.NewComponentType[PositionComponent]()
