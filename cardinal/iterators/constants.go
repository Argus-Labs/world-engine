package iterators

import (
	"errors"

	"pkg.world.dev/world-engine/cardinal/types"
)

var (
	BadID types.EntityID = ""

	ErrEntityDoesNotExist                = errors.New("entity does not exist")
	ErrComponentAlreadyOnEntity          = errors.New("component already on entity")
	ErrComponentNotOnEntity              = errors.New("component not on entity")
	ErrEntityMustHaveAtLeastOneComponent = errors.New("entities must have at least 1 component")
	ErrMustRegisterComponent             = errors.New("must register component")

	// ErrComponentMismatchWithSavedState is an error that is returned when a ComponentID from
	// the saved state is not found in the passed in list of components.
	ErrComponentMismatchWithSavedState = errors.New("registered components do not match with the saved state")
)
