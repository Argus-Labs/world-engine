package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type and struct {
	filters []ComponentFilter
}

func And(filters ...ComponentFilter) ComponentFilter {
	return &and{filters: filters}
}

func (f *and) MatchesComponents(components []component.IComponentType) bool {
	for _, filter := range f.filters {
		if !filter.MatchesComponents(components) {
			return false
		}
	}
	return true
}
