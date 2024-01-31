package cardinal

import (
	filter2 "pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/systems"
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
	All      = filter2.All
	And      = filter2.And
	Or       = filter2.Or
	Not      = filter2.Not
	Contains = filter2.Contains
	Exact    = filter2.Exact
)
