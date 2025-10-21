package ecs

import "github.com/rotisserie/eris"

var (
	// ErrEntityNotFound is returned when attempting to operate on a non-existent entity
	// or when an entity cannot be found in the expected location.
	ErrEntityNotFound = eris.New("entity does not exist")
)
