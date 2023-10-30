package filter

import "pkg.world.dev/world-engine/cardinal/ecs/component/metadata"

type contains struct {
	components []metadata.ComponentMetadata
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...metadata.ComponentMetadata) ComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []metadata.ComponentMetadata) bool {
	for _, componentType := range f.components {
		if !MatchComponentMetaData(components, componentType) {
			return false
		}
	}
	return true
}
