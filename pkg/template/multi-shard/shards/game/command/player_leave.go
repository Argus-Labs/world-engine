package command

import "github.com/argus-labs/world-engine/pkg/cardinal"

type PlayerLeave struct {
	cardinal.BaseCommand
	ArgusAuthID string `json:"argus_auth_id"`
}

func (p PlayerLeave) Name() string {
	return "player-leave"
}
