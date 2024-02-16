package msg

import (
	"pkg.world.dev/world-engine/cardinal/message"
)

type JoinInput struct {
	Ok bool
}

type JoinOutput struct {
	Success bool
}

var JoinMsg = message.NewMessageType[JoinInput, JoinOutput]("join",
	message.WithMsgEVMSupport[JoinInput, JoinOutput]())
