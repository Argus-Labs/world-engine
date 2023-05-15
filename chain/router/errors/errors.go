package errors

import "fmt"

const (
	Success = iota
	Failed
)

var (
	ErrNamespaceNotFound = func(namespace string) error {
		return fmt.Errorf("namespace %s not found", namespace)
	}
)
