package gamestate

import (
	"github.com/rotisserie/eris"
)

var (
	ErrEntityDoesNotExist                = eris.New("entity does not exist")
	ErrComponentAlreadyOnEntity          = eris.New("component already on entity")
	ErrComponentNotOnEntity              = eris.New("component not on entity")
	ErrEntityMustHaveAtLeastOneComponent = eris.New("entities must have at least 1 component")
	ErrComponentNotRegistered            = eris.New("must register component")

	// ErrComponentMismatchWithSavedState is an error that is returned when a ComponentID from
	// the saved state is not found in the passed in list of components.
	ErrComponentMismatchWithSavedState = eris.New("registered components do not match with the saved state")
)
