package filter

import (
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type or struct {
	filters []interfaces.IComponentFilter
}

func Or(filters ...interfaces.IComponentFilter) interfaces.IComponentFilter {
	return &or{filters: filters}
}

func (f *or) MatchesComponents(components []interfaces.IComponentType) bool {
	for _, filter := range f.filters {
		if filter.MatchesComponents(components) {
			return true
		}
	}
	return false
}
