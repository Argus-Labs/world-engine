package component

import "github.com/argus-labs/world-engine/cardinal/ecs"

type HealthComponent struct {
	Val int
}

type PositionComponent struct {
	X, Y int
}

var (
	Health   = ecs.NewComponentType[HealthComponent]()
	Position = ecs.NewComponentType[PositionComponent]()
)

func MustInitialize(world *ecs.World) {
	err := world.RegisterComponents(
		Health,
		Position,
	)
	if err != nil {
		panic(err)
	}
}
