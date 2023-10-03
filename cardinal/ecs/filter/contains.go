package filter

import "pkg.world.dev/world-engine/cardinal/public"

type contains struct {
	components []public.IComponentType
}

// Contains matches archetypes that contain all the components specified.
func Contains(components ...public.IComponentType) public.IComponentFilter {
	return &contains{components: components}
}

func (f *contains) MatchesComponents(components []public.IComponentType) bool {
	for _, componentType := range f.components {
		if !containsComponent(components, componentType) {
			return false
		}
	}
	return true
}
