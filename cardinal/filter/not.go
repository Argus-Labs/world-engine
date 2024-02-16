package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

func Not(filter ComponentFilter) ComponentFilter {
	return &not{filter: filter}
}

type not struct {
	filter ComponentFilter
}

func (f *not) MatchesComponents(components []types.Component) bool {
	return !f.filter.MatchesComponents(components)
}
