package component

type Health struct {
	HP int
}

func (Health) Name() string {
	return "Health"
}
