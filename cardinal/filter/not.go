package filter

import "pkg.world.dev/world-engine/cardinal/public"

type not struct {
	filter public.IComponentFilter
}

func Not(filter public.IComponentFilter) public.IComponentFilter {
	return &not{filter: filter}
}

func (f *not) MatchesComponents(components []public.IComponentType) bool {
	return !f.filter.MatchesComponents(components)
}
