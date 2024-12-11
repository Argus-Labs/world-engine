package testsuite

// LocationComponent is a test component for location-based tests
type LocationComponent struct {
	X, Y uint64
}

func (l LocationComponent) Name() string {
	return "location"
}

// ValueComponent is a test component for value-based tests
type ValueComponent struct {
	Value int64
}

func (v ValueComponent) Name() string {
	return "value"
}

// PowerComponent is a test component for power-based tests
type PowerComponent struct {
	Power int64
}

func (p PowerComponent) Name() string {
	return "power"
}
