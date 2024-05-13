package types

import "fmt"

type QueryHandler = func(name string, group string, bz []byte) ([]byte, error)

type QueryNotFoundError struct {
	name string
}

func NewQueryNotFoundError(name string) *QueryNotFoundError {
	return &QueryNotFoundError{name: name}
}

func (e *QueryNotFoundError) Error() string {
	return fmt.Sprintf("could not find query with name: %s", e.name)
}
