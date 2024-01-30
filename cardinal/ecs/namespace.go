package ecs

// Namespace is a unique identifier for a engine.
type Namespace string

func (n Namespace) String() string {
	return string(n)
}
