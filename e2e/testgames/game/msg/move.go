package msg

import (
	"pkg.world.dev/world-engine/cardinal/message"
)

type MoveInput struct {
	Direction string `json:"direction"`
}

type MoveOutput struct {
	X, Y int64
}

var MoveMsg = message.NewMessageType[MoveInput, MoveOutput]("move", message.WithMsgEVMSupport[MoveInput, MoveOutput]())
