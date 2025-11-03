package event

import "github.com/argus-labs/world-engine/pkg/cardinal"

type PlayerDeath struct {
	cardinal.BaseEvent
	Nickname string
}

func (PlayerDeath) Name() string {
	return "player-death"
}

type NewPlayer struct {
	cardinal.BaseEvent
	Nickname string
}

func (NewPlayer) Name() string {
	return "new-player"
}
