package filter

import "pkg.world.dev/world-engine/cardinal/types/component"

type exact struct {
	components []component.ComponentMetadata
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...component.ComponentMetadata) ComponentFilter {
	return exact{
		components: components,
	}
}

func (f exact) MatchesComponents(components []component.ComponentMetadata) bool {
	if len(components) != len(f.components) {
		return false
	}
	for _, componentType := range components {
		if !MatchComponentMetaData(f.components, componentType) {
			return false
		}
	}
	return true
}
