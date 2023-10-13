package filter

import (
	"pkg.world.dev/world-engine/cardinal/ecs/component"
)

type not struct {
	filter ComponentFilter
}

func Not(filter ComponentFilter) ComponentFilter {
	return &not{filter: filter}
}

func (f *not) MatchesComponents(components []component.IComponentMetaData) bool {
	return !f.filter.MatchesComponents(components)
}
