package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/icomponent"
)

type not struct {
	filter ComponentFilter
}

func Not(filter ComponentFilter) ComponentFilter {
	return &not{filter: filter}
}

func (f *not) MatchesComponents(components []icomponent.IComponentType) bool {
	return !f.filter.MatchesComponents(components)
}
