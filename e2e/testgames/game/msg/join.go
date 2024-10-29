package msg

type JoinInput struct {
	Ok bool
}

func (JoinInput) Name() string {
	return "join"
}
