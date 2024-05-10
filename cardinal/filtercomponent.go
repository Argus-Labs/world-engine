package cardinal

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
)

// This package involves primitives for search.
// It involves creating and combining primitives that represent
// filtering properties on components.

type FilterFn func(wCtx Context, id types.EntityID) (bool, error)

//revive:disable-next-line:unexported-return
func ComponentFilter[T types.Component](f func(comp T) bool) FilterFn {
	return func(wCtx Context, id types.EntityID) (bool, error) {
		var t T
		c, err := wCtx.GetComponentByName(t.Name())
		if err != nil {
			return false, err
		}
		// Get current component value
		compValue, err := wCtx.storeReader().GetComponentForEntity(c, id)
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

//revive:disable-next-line:unexported-return
func AndFilter(fns ...FilterFn) FilterFn {
	return func(wCtx Context, id types.EntityID) (bool, error) {
		var result = true
		var errCount = 0
		for _, fn := range fns {
			res, err := fn(wCtx, id)
			if err != nil {
				errCount++
				continue
			}
			result = result && res
		}
		if errCount == len(fns) {
			return false, eris.New("all filters failed")
		}
		return result, nil
	}
}

//revive:disable-next-line:unexported-return
func OrFilter(fns ...FilterFn) FilterFn {
	return func(wCtx Context, id types.EntityID) (bool, error) {
		var result = false
		var errCount = 0
		for _, fn := range fns {
			res, err := fn(wCtx, id)
			if err != nil {
				errCount++
				continue
			}
			result = result || res
		}
		if errCount == len(fns) {
			return false, eris.New("all filters failed")
		}
		return result, nil
	}
}
