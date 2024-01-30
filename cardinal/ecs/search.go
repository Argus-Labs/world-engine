package ecs

import (
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/search"
)

func (e *Engine) NewSearch(filter filter.ComponentFilter) *search.Search {
	// TODO(scott): .toReadOnly() seems to break the search. investigate.
	return search.NewSearch(filter, string(e.namespace), e.GameStateManager())
}
