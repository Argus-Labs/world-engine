package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type or struct {
	filters []ComponentFilter
}

func Or(filters ...ComponentFilter) ComponentFilter {
	return &or{filters: filters}
}

func (f *or) MatchesComponents(components []component.IComponentType) bool {
	for _, filter := range f.filters {
		if filter.MatchesComponents(components) {
			return true
		}
	}
	return false
}
