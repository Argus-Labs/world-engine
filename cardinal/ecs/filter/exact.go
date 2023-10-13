package filter

import "pkg.world.dev/world-engine/cardinal/ecs/component_metadata"

type exact struct {
	components []component_metadata.IComponentMetaData
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...component_metadata.IComponentMetaData) ComponentFilter {
	return exact{
		components: components,
	}
}

func (f exact) MatchesComponents(components []component_metadata.IComponentMetaData) bool {
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
