package component

type Level struct {
	Value int
}

func (Level) Name() string {
	return "Level"
}
