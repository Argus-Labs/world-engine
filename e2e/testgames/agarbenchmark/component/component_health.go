package component

type Health struct {
	Value int
}

func (Health) Name() string {
	return "Health"
}

func (health *Health) IsAlive() bool {
	return health.Value > 0
}
