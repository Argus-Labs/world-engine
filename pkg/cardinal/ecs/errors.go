package ecs

import "github.com/rotisserie/eris"

var (
	// ErrEntityNotFound is returned when attempting to operate on a non-existent entity
	// or when an entity cannot be found in the expected location.
	ErrEntityNotFound = eris.New("entity does not exist")

	// ErrComponentNotFound is returned when attempting to operate on a component that isn't
	// registered (used) in any systems.
	ErrComponentNotFound = eris.New("component is not registered")
)
