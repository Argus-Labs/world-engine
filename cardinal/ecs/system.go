package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/climate"
)

type System func(climate.Climate) error
