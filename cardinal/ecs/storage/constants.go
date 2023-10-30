package storage

import (
	"errors"
	"math"

	"pkg.world.dev/world-engine/cardinal/ecs/entity"
)

var (
	BadID entity.ID = math.MaxUint64

	ErrComponentAlreadyOnEntity          = errors.New("component already on entity")
	ErrComponentNotOnEntity              = errors.New("component not on entity")
	ErrEntityMustHaveAtLeastOneComponent = errors.New("entities must have at least 1 component")

	// ErrComponentMismatchWithSavedState is an error that is returned when a TypeID from
	// the saved state is not found in the passed in list of components.
	ErrComponentMismatchWithSavedState = errors.New("registered components do not match with the saved state")
)
