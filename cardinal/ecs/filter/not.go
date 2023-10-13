package filter

import "pkg.world.dev/world-engine/cardinal/ecs/component_metadata"

func Not(filter ComponentFilter) ComponentFilter {
	return &not{filter: filter}
}

type not struct {
	filter ComponentFilter
}

func (f *not) MatchesComponents(components []component_metadata.IComponentMetaData) bool {
	return !f.filter.MatchesComponents(components)
}
