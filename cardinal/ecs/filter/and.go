package filter

import (
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type and struct {
	filters []interfaces.IComponentFilter
}

func And(filters ...interfaces.IComponentFilter) interfaces.IComponentFilter {
	return &and{filters: filters}
}

func (f *and) MatchesComponents(components []interfaces.IComponentType) bool {
	for _, filter := range f.filters {
		if !filter.MatchesComponents(components) {
			return false
		}
	}
	return true
}
