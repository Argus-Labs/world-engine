package event

import "github.com/argus-labs/world-engine/pkg/cardinal"

// New player has joined the quadrant

type PlayerSpawn struct {
	cardinal.BaseEvent
	ArgusAuthID   string `json:"argus_auth_id"`
	ArgusAuthName string `json:"argus_auth_name"`
	X             uint32 `json:"x"`
	Y             uint32 `json:"y"`
}

func (PlayerSpawn) Name() string {
	return "player-spawn"
}

// Player has moved inside the quadrant

type PlayerMovement struct {
	cardinal.BaseEvent
	ArgusAuthID   string `json:"argus_auth_id"`
	ArgusAuthName string `json:"argus_auth_name"`
	X             uint32 `json:"x"`
	Y             uint32 `json:"y"`
}

func (PlayerMovement) Name() string {
	return "player-movement"
}

// Player has left the quadrant (either by leaving the quadrant or going offline)

type PlayerDeparture struct {
	cardinal.BaseEvent
	ArgusAuthID string `json:"argus_auth_id"`
}

func (PlayerDeparture) Name() string {
	return "player-departure"
}
