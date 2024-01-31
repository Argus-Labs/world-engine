package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/search"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

func NewSearch(wCtx engine.Context, filter filter.ComponentFilter) *search.Search {
	return search.NewSearch(filter, wCtx.Namespace(), wCtx.StoreReader())
}
