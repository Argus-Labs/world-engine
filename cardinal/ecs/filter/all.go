package filter

import "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

type all struct {
}

func All() ComponentFilter {
	return &all{}
}

func (f *all) MatchesComponents(_ []metadata.ComponentMetadata) bool {
	return true
}
