package component

type Health struct {
	HP int `json:"hp"`
}

func (Health) Name() string {
	return "health"
}
