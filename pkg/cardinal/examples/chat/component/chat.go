package component

import "time"

type Chat struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func (Chat) Name() string {
	return "chat"
}
