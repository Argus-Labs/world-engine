package msg

import (
	"pkg.world.dev/world-engine/cardinal"
)

type CreatePlayerMsg struct {
	Nickname string `json:"nickname"`
}

type CreatePlayerResult struct {
	Success bool `json:"success"`
}

var CreatePlayer = cardinal.NewMessageType[CreatePlayerMsg, CreatePlayerResult]("create-player")
