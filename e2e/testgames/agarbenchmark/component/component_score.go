package component

type Score struct {
	Value int
}

func (Score) Name() string {
	return "Score"
}
