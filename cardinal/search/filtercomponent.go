package search

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

// This package involves primitives for search.
// It involves creating and combining primitives that represent
// filtering properties on components.

// PredicateEvaluator is an interface used as input into the where clause for search.
// if the Evaluate function returns true without an error the search will enable that component for the id.
// if an error is returned the search will treat the evaluation as if it didn't exist.
type PredicateEvaluator interface {
	Evaluate(wCtx engine.Context, id types.EntityID) (bool, error)
}

type componentFilter[T types.Component] struct {
	FilterFunc func(comp T) bool
}

type andFilterComponent struct {
	filterComponents []PredicateEvaluator
}

type orFilterComponent struct {
	filterComponents []PredicateEvaluator
}

type notFilterComponent struct {
	filterComponent PredicateEvaluator
}

func FilterFunction[T types.Component](f func(comp T) bool) PredicateEvaluator {
	return &componentFilter[T]{
		FilterFunc: f,
	}
}

func (afc *andFilterComponent) Evaluate(wCtx engine.Context, id types.EntityID) (bool, error) {
	var result = true
	errCount := 0
	for _, filterComp := range afc.filterComponents {
		otherResult, err := filterComp.Evaluate(wCtx, id)
		if err != nil {
			errCount++
			continue
		}
		result = result && otherResult
		if !result {
			break
		}
	}
	if errCount == len(afc.filterComponents) {
		return false, eris.New("all filter evaluations invalid")
	}
	return result, nil
}

func (nfc *notFilterComponent) Evaluate(wCtx engine.Context, id types.EntityID) (bool, error) {
	result, err := nfc.filterComponent.Evaluate(wCtx, id)
	if err != nil {
		return result, err
	}
	return !result, nil
}

func (ofc *orFilterComponent) Evaluate(wCtx engine.Context, id types.EntityID) (bool, error) {
	var result = true
	errCount := 0
	for _, filterComp := range ofc.filterComponents {
		otherResult, err := filterComp.Evaluate(wCtx, id)
		if err != nil {
			errCount++
			continue
		}
		result = result || otherResult
		if result {
			break
		}
	}
	if errCount == len(ofc.filterComponents) {
		return false, eris.New("all filter evaluations invalid")
	}
	return result, nil
}

// Evaluate when given an entity id will attempt to fetch the related component on that entity.
// if the entity fails to get retrieved Evaluate will return an error. The overall search algorithm
// will interpret this error as a command to pretend that this filter doesn't exist all together.
// if the component exists it will be passed to the predicate function located within component filter and evaluate
// to either true or false.
func (fc *componentFilter[T]) Evaluate(wCtx engine.Context, id types.EntityID) (bool, error) {
	// Get the component metadata
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
	return fc.FilterFunc(*comp), nil
}
