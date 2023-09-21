package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type not struct {
	filter ComponentFilter
}

func Not(filter ComponentFilter) ComponentFilter {
	return &not{filter: filter}
}

func (f *not) MatchesComponents(components []component.IComponentType) bool {
	return !f.filter.MatchesComponents(components)
}
