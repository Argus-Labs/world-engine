package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

// ComponentFilter is a filter that filters entities based on their components.
type ComponentFilter interface {
	// MatchesComponents returns true if the entity matches the filter.
	MatchesComponents(components []types.Component) bool
}

type ComponentWrapper struct {
	Component types.Component
}

// Component is public but contains an unexported return type
// this is done with intent as the user should never use ComponentWrapper
// explicitly.
//
//revive:disable-next-line:unexported-return
func Component[T types.Component]() ComponentWrapper {
	var x T
	return ComponentWrapper{
		Component: x,
	}
}
