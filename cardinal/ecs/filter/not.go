package filter

import component_metadata "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

func Not(filter ComponentFilter) ComponentFilter {
	return &not{filter: filter}
}

type not struct {
	filter ComponentFilter
}

func (f *not) MatchesComponents(components []component_metadata.ComponentMetadata) bool {
	return !f.filter.MatchesComponents(components)
}
