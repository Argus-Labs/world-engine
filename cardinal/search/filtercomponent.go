package search

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// This package involves primitives for search.
// It involves creating and combining primitives that represent
// filtering properties on components.

type filterFn func(wCtx engine.Context, id types.EntityID) (bool, error)

func ComponentFilter[T types.Component](f func(comp T) bool) filterFn {
	return func(wCtx engine.Context, id types.EntityID) (bool, error) {
		var t T
		c, err := wCtx.GetComponentByName(t.Name())
		if err != nil {
			return false, err
		}
		// Get current component value
		compValue, err := wCtx.StoreReader().GetComponentForEntity(c, id)
		if err != nil {
			return false, err
		}

		// Type assert the component value to the component type
		var comp *T
		t, ok := compValue.(T)
		if !ok {
			comp, ok = compValue.(*T)
			if !ok {
				return false, eris.New("no result found.")
			}
		} else {
			comp = &t
		}
		return f(*comp), nil
	}
}

func AndFilter(fns ...filterFn) filterFn {
	return func(wCtx engine.Context, id types.EntityID) (bool, error) {
		var result bool = true
		var errCount = 0
		for _, fn := range fns {
			res, err := fn(wCtx, id)
			if err != nil {
				continue
				errCount++
			}
			result = result && res
		}
		if errCount == len(fns) {
			return false, eris.New("all filters failed")
		}
		return result, nil
	}
}

func OrFilter(fns ...filterFn) filterFn {
	return func(wCtx engine.Context, id types.EntityID) (bool, error) {
		var result bool = false
		var errCount = 0
		for _, fn := range fns {
			res, err := fn(wCtx, id)
			if err != nil {
				continue
				errCount++
			}
			result = result || res
		}
		if errCount == len(fns) {
			return false, eris.New("all filters failed")
		}
		return result, nil
	}
}
