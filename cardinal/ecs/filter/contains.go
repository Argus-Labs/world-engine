package filter

import "pkg.world.dev/world-engine/cardinal/types/component"

type contains struct {
	components []component.ComponentMetadata
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...component.ComponentMetadata) ComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []component.ComponentMetadata) bool {
	for _, componentType := range f.components {
		if !MatchComponentMetaData(components, componentType) {
			return false
		}
	}
	return true
}
