package filter

import (
	"pkg.world.dev/world-engine/cardinal/v2/types"
)

type all struct {
}

func All() ComponentFilter {
	return &all{}
}

func (f *all) MatchesComponents(_ []types.Component) bool {
	return true
}
