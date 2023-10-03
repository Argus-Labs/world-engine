package filter

import "pkg.world.dev/world-engine/cardinal/public"

func containsComponent(components []public.IComponentType, c public.IComponentType) bool {
	for _, comp := range components {
		if comp == c {
			return true
		}
	}
	return false
}
