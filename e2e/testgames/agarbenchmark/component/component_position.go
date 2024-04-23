package component

type Position struct {
	X float64
	Y float64
}

func (Position) Name() string {
	return "Position"
}
