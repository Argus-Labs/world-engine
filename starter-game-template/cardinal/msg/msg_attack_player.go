package msg

import (
	"pkg.world.dev/world-engine/cardinal"
)

type AttackPlayerMsg struct {
	TargetNickname string `json:"target"`
}

type AttackPlayerMsgReply struct {
	Damage int `json:"damage"`
}

var AttackPlayer = cardinal.NewMessageType[AttackPlayerMsg, AttackPlayerMsgReply]("attack-player")
