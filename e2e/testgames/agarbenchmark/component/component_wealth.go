package component

type Wealth struct {
	Value int
}

func (Wealth) Name() string {
	return "Wealth"
}
