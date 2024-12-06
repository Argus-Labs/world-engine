package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

// ComponentFilter is a filter that filters entities based on their components.
type ComponentFilter interface {
	// MatchesComponents returns true if the entity matches the filter.
	MatchesComponents(components []types.Component) bool
}

// ComponentWrapper wraps a Component type for filtering purposes.
type ComponentWrapper struct {
	Component types.Component
}

// Component returns a ComponentWrapper for the given component type T.
// This function is intentionally designed to return an unexported type
// as ComponentWrapper should not be used directly.
func Component[T types.Component]() ComponentWrapper {
	var x T
	return ComponentWrapper{
		Component: x,
	}
}
