package system

import (
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type System func(ctx engine.Context) error
