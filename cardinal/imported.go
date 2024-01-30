package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type WorldContext = engine.Context

var All = filter.All
var And = filter.And
var Or = filter.Or
var Not = filter.Not
var Contains = filter.Contains
var Exact = filter.Exact
