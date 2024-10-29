package msg

type MoveInput struct {
	Direction string `json:"direction"`
}

func (MoveInput) Name() string {
	return "move"
}
