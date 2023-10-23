package comp

type Location struct {
	X, Y int64
}

func (l Location) Name() string {
	return "location"
}
