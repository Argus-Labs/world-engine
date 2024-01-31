package filter

import "pkg.world.dev/world-engine/cardinal/types/component"

type exact struct {
	components []component.Component
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...component.Component) ComponentFilter {
	return exact{
		components: components,
	}
}

func (f exact) MatchesComponents(components []component.Component) bool {
	if len(components) != len(f.components) {
		return false
	}
	for _, componentType := range components {
		if !MatchComponent(f.components, componentType) {
			return false
		}
	}
	return true
}
