package command

import "github.com/argus-labs/world-engine/pkg/cardinal"

type MovePlayer struct {
	cardinal.BaseCommand
	ArgusAuthID string `json:"argus_auth_id"`
	X           uint32 `json:"x"`
	Y           uint32 `json:"y"`
}

func (a MovePlayer) Name() string {
	return "move-player"
}
