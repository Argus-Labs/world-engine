package search

import "pkg.world.dev/world-engine/cardinal/types"

type SearchComponent struct {
	Component types.Component
}

func Component[T types.Component]() SearchComponent {
	var x T
	return SearchComponent{
		Component: x,
	}
}
