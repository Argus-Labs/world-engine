package component

type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

func (Position) Name() string {
	return "position"
}
