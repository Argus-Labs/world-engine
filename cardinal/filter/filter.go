package filter

import (
	"pkg.world.dev/world-engine/cardinal/types"
)

// ComponentFilter is a filter that filters entities based on their components.
type ComponentFilter interface {
	// MatchesComponents returns true if the entity matches the filter.
	MatchesComponents(components []types.Component) bool
}
