package filter

import "pkg.world.dev/world-engine/cardinal/public"

type exact struct {
	components []public.IComponentType
}

// Exact matches archetypes that contain exactly the same components specified.
func Exact(components ...public.IComponentType) public.IComponentFilter {
	return exact{
		components: components,
	}
}

func (f exact) MatchesComponents(components []public.IComponentType) bool {
	if len(components) != len(f.components) {
		return false
	}
	for _, componentType := range components {
		if !containsComponent(f.components, componentType) {
			return false
		}
	}
	return true
}
