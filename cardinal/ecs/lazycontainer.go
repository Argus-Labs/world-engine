package ecs

// Lazy container can be moved to a package later.
type LazyContainer[T any] struct {
	Unbox func() (T, error)
}

func NewLazyContainer[T any](unboxMethod func() (T, error)) LazyContainer[T] {
	return LazyContainer[T]{
		Unbox: unboxMethod,
	}
}
