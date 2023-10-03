package filter

import (
	"pkg.world.dev/world-engine/cardinal/interfaces"
)

type not struct {
	filter interfaces.IComponentFilter
}

func Not(filter interfaces.IComponentFilter) interfaces.IComponentFilter {
	return &not{filter: filter}
}

func (f *not) MatchesComponents(components []interfaces.IComponentType) bool {
	return !f.filter.MatchesComponents(components)
}
