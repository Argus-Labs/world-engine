package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/search"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

func NewSearch(eCtx engine.Context, filter filter.ComponentFilter) *search.Search {
	return search.NewSearch(filter, eCtx.Namespace(), eCtx.StoreReader())
}
