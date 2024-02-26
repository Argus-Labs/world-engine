package msg

type MoveInput struct {
	Direction string `json:"direction"`
}

type MoveOutput struct {
	X, Y int64
}
