package search

import "pkg.world.dev/world-engine/cardinal/types"

// This file represents primitives for search
// The primitive wraps the component and is used in search
// the purpose of the wrapper is to prevent the user from ever
// instantiating the component.

type componentWrapper struct {
	Component types.Component
}

func Component[T types.Component]() componentWrapper {
	var x T
	return componentWrapper{
		Component: x,
	}
}
