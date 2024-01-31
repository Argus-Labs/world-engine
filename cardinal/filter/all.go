package filter

import "pkg.world.dev/world-engine/cardinal/types/component"

type all struct {
}

func All() ComponentFilter {
	return &all{}
}

func (f *all) MatchesComponents(_ []component.Component) bool {
	return true
}
