package storage

import (
	"errors"
	"math"

	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

var (
	BadID entity.ID = math.MaxUint64

	ErrorComponentAlreadyOnEntity          = errors.New("component already on entity")
	ErrorComponentNotOnEntity              = errors.New("component not on entity")
	ErrorEntityMustHaveAtLeastOneComponent = errors.New("entities must have at least 1 component")

	// ErrorComponentMismatchWithSavedState is an error that is returned when a TypeID from
	// the saved state is not found in the passed in list of components.
	ErrorComponentMismatchWithSavedState = errors.New("registered components do not match with the saved state")
)
