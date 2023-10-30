package filter

import component_metadata "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

type and struct {
	filters []ComponentFilter
}

func And(filters ...ComponentFilter) ComponentFilter {
	return &and{filters: filters}
}

func (f *and) MatchesComponents(components []component_metadata.ComponentMetadata) bool {
	for _, filter := range f.filters {
		if !filter.MatchesComponents(components) {
			return false
		}
	}
	return true
}
