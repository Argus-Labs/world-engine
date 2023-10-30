package filter

import component_metadata "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

type exact struct {
	components []component_metadata.ComponentMetadata
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...component_metadata.ComponentMetadata) ComponentFilter {
	return exact{
		components: components,
	}
}

func (f exact) MatchesComponents(components []component_metadata.ComponentMetadata) bool {
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
