package testsuite

// LocationComponent is a test component for location-based tests
type LocationComponent struct {
	X, Y uint64
}

func (LocationComponent) Name() string {
	return "location"
}
