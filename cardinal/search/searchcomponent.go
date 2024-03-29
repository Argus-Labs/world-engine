package search

import "pkg.world.dev/world-engine/cardinal/types"

type ComponentWrapper struct {
	Component types.Component
}

func Component[T types.Component]() ComponentWrapper {
	var x T
	return ComponentWrapper{
		Component: x,
	}
}
