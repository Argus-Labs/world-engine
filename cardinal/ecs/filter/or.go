package filter

import "pkg.world.dev/world-engine/cardinal/public"

type or struct {
	filters []public.IComponentFilter
}

func Or(filters ...public.IComponentFilter) public.IComponentFilter {
	return &or{filters: filters}
}

func (f *or) MatchesComponents(components []public.IComponentType) bool {
	for _, filter := range f.filters {
		if filter.MatchesComponents(components) {
			return true
		}
	}
	return false
}
