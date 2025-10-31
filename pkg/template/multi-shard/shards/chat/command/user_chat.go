package command

import "github.com/argus-labs/world-engine/pkg/cardinal"

type UserChat struct {
	cardinal.BaseCommand
	ArgusAuthID   string `json:"argus_auth_id"`
	ArgusAuthName string `json:"argus_auth_name"`
	Message       string `json:"message"`
}

func (UserChat) Name() string {
	return "user-chat"
}
