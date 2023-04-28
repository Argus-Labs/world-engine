package ecs

func Contains[T any](collection []T, item T, equal func(x, y T) bool) bool {
	for _, i := range collection {
		if equal(i, item) {
			return true
		}
	}
	return false
}
