package filter

import "pkg.world.dev/world-engine/cardinal/public"

type and struct {
	filters []public.IComponentFilter
}

func And(filters ...public.IComponentFilter) public.IComponentFilter {
	return &and{filters: filters}
}

func (f *and) MatchesComponents(components []public.IComponentType) bool {
	for _, filter := range f.filters {
		if !filter.MatchesComponents(components) {
			return false
		}
	}
	return true
}
