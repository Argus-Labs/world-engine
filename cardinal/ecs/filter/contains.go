package filter

import component_metadata "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

type contains struct {
	components []component_metadata.ComponentMetadata
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...component_metadata.ComponentMetadata) ComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []component_metadata.ComponentMetadata) bool {
	for _, componentType := range f.components {
		if !MatchComponentMetaData(components, componentType) {
			return false
		}
	}
	return true
}
