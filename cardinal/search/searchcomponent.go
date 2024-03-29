package search

import "pkg.world.dev/world-engine/cardinal/types"

// This file represents primitives for search
// The primitive wraps the component and is used in search
// the purpose of the wrapper is to prevent the user from ever
// instantiating the component. These wrappers are used to check if entities
// contain the specified component during the search.

type componentWrapper struct {
	Component types.Component
}

// Component is public but contains an unexported return type
// this is done with intent as the user should never use componentWrapper
// explicitly.
func Component[T types.Component]() componentWrapper {
	var x T
	return componentWrapper{
		Component: x,
	}
}
