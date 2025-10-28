package component

import "time"

type OnlineStatus struct {
	Online     bool      `json:"online"`
	LastActive time.Time `json:"last_active"`
}

func (OnlineStatus) Name() string {
	return "onlinestatus"
}
