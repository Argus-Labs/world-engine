package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/system"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

var (
	ErrEntityDoesNotExist                = gamestate.ErrEntityDoesNotExist
	ErrEntityMustHaveAtLeastOneComponent = gamestate.ErrEntityMustHaveAtLeastOneComponent
	ErrComponentNotOnEntity              = gamestate.ErrComponentNotOnEntity
	ErrComponentAlreadyOnEntity          = gamestate.ErrComponentAlreadyOnEntity
)

type WorldContext = engine.Context

type System = system.System
