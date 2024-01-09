package cardinal

// Lazy container stores a closure that evaluates to T.
type LazyContainer[T any] struct {
	Unbox func() (T, error)
}

func NewLazyContainer[T any](unboxMethod func() (T, error)) LazyContainer[T] {
	return LazyContainer[T]{
		Unbox: unboxMethod,
	}
}
