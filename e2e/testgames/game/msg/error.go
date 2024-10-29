package msg

type ErrorInput struct {
	ErrorMsg string
}

func (ErrorInput) Name() string {
	return "error"
}
