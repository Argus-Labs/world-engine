package search

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
)

type FilterFn func(id types.EntityID) (bool, error)

func andFilter(fns ...FilterFn) FilterFn {
	return func(id types.EntityID) (bool, error) {
		for _, fn := range fns {
			passedFilter, err := fn(id)
			if err != nil {
				return false, eris.Wrap(err, "an error occured while evaluating a filter")
			}

			// In an andFilter, if any of the filters return false, the whole filter returns false.
			if !passedFilter {
				return false, nil
			}
		}

		return true, nil
	}
}

func orFilter(fns ...FilterFn) FilterFn {
	return func(id types.EntityID) (bool, error) {
		for _, fn := range fns {
			passedFilter, err := fn(id)
			if err != nil {
				return false, eris.Wrap(err, "an error occured while evaluating a filter")
			}

			// In an orFilter, if any of the filters return true, the whole filter returns true.
			if passedFilter {
				return true, nil
			}
		}

		return false, nil
	}
}
