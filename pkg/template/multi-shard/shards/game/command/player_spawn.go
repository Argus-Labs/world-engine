package command

import (
	"github.com/argus-labs/world-engine/pkg/cardinal"
)

type PlayerSpawn struct {
	cardinal.BaseCommand
	ArgusAuthID   string `json:"argus_auth_id"`
	ArgusAuthName string `json:"argus_auth_name"`
	X             uint32 `json:"x"`
	Y             uint32 `json:"y"`
}

func (a PlayerSpawn) Name() string {
	return "player-spawn"
}
