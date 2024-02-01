package msg

import (
	"pkg.world.dev/world-engine/cardinal"
)

type MoveInput struct {
	Direction string `json:"direction"`
}

type MoveOutput struct {
	X, Y int64
}

var MoveMsg = cardinal.NewMessageType[MoveInput, MoveOutput]("move",
	cardinal.WithMsgEVMSupport[MoveInput, MoveOutput]())
