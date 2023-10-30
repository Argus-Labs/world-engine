package filter

import component_metadata "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

type contains struct {
	components []component_metadata.IComponentMetaData
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...component_metadata.IComponentMetaData) ComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []component_metadata.IComponentMetaData) bool {
	for _, componentType := range f.components {
		if !MatchComponentMetaData(components, componentType) {
			return false
		}
	}
	return true
}
