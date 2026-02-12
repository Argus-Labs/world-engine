package event

import (
	"time"
)

// Someone typed a message in chat

type UserChat struct {
	ArgusAuthID   string    `json:"argus_auth_id"`
	ArgusAuthName string    `json:"argus_auth_name"`
	Message       string    `json:"message"`
	Timestamp     time.Time `json:"timestamp"`
}

func (UserChat) Name() string {
	return "user-chat"
}
