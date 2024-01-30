package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/systems"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/cardinal/types/message"
)

type (
	// EntityID represents a single entity in the World. An EntityID is tied to
	// one or more components.
	EntityID     = entity.ID
	TxHash       = message.TxHash
	Receipt      = receipt.Receipt
	System       = systems.System
	WorldContext = engine.Context
)

var (
	All      = filter.All
	And      = filter.And
	Or       = filter.Or
	Not      = filter.Not
	Contains = filter.Contains
	Exact    = filter.Exact
)
