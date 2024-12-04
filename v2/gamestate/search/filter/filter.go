package filter

import (
	"pkg.world.dev/world-engine/cardinal/v2/types"
)

// ComponentFilter is a filter that filters entities based on their components.
type ComponentFilter interface {
	// MatchesComponents returns true if the entity matches the filter.
	MatchesComponents(components []types.Component) bool
}

type componentWrapper struct {
	types.Component
	name string
}

var _ types.Component = componentWrapper{}

func (c componentWrapper) Name() string {
	return c.name
}

// Component is public but contains an unexported return type
// this is done with intent as the user should never use componentWrapper
// explicitly.
//
//revive:disable-next-line:unexported-return
func Component[T types.Component]() componentWrapper {
	var t T
	return componentWrapper{
		name: t.Name(),
	}
}

func ComponentWithName(name string) componentWrapper {
	return componentWrapper{
		name: name,
	}
}
