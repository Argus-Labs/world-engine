package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
)

type CardinalSpecificWorldContextMethods interface {
	NewSearch(filter CardinalFilter) (*Search, error)
	GetWorld() *World
}

type WorldContext interface {
	CardinalSpecificWorldContextMethods
	ecs.GeneralWorldContextMethods
}

type CardinalWorldContextStruct struct {
	implWorld   *World
	implContext ecs.WorldContext
}

func (wCtx *CardinalWorldContextStruct) NewSearch(filter CardinalFilter) (*ecs.Search, error) {
	return wCtx.implContext.NewSearch(filter.ConvertToFilterable())
}

func (wCtx *CardinalWorldContextStruct) GetWorld() *World {
	return wCtx.implWorld
}
