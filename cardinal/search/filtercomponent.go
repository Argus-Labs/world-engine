package search

import (
	"fmt"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

func FilterFunction[T types.Component](f func(comp T) bool) PredicateEvaluator {
	return &ComponentFilter[T]{
		FilterFunc: f,
	}
}

type PredicateEvaluator interface {
	Evaluate(wCtx engine.Context, id types.EntityID) (bool, error)
}

type ComponentFilter[T types.Component] struct {
	FilterFunc func(comp T) bool
}

type andedFilterComponent struct {
	filterComponents []PredicateEvaluator
}

func (afc *andedFilterComponent) Evaluate(wCtx engine.Context, id types.EntityID) (bool, error) {
	var result = true
	for _, filterComp := range afc.filterComponents {
		otherResult, err := filterComp.Evaluate(wCtx, id)
		if err != nil {
			continue
		}
		result = result && otherResult
		if result != true {
			break
		}
	}
	return result, nil
}

type oredFilterComponent struct {
	filterComponents []PredicateEvaluator
}

type notFilterComponent struct {
	filterComponent PredicateEvaluator
}

func (nfc *notFilterComponent) Evaluate(wCtx engine.Context, id types.EntityID) (bool, error) {
	result, err := nfc.Evaluate(wCtx, id)
	if err != nil {
		return false, err
	}
	return result, nil
}

func (ofc *oredFilterComponent) Evaluate(wCtx engine.Context, id types.EntityID) (bool, error) {
	var result = true
	for _, filterComp := range ofc.filterComponents {
		otherResult, err := filterComp.Evaluate(wCtx, id)
		if err != nil {
			continue
		}
		result = result || otherResult
		if result == true {
			break
		}
	}
	return result, nil
}

func (fc *ComponentFilter[T]) Evaluate(wCtx engine.Context, id types.EntityID) (bool, error) {
	// Get the component metadata
	var t T
	fmt.Println(t.Name())
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
	if err != nil {
		return false, err
	}
	return fc.FilterFunc(*comp), nil
}
