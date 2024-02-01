package msg

import (
	"pkg.world.dev/world-engine/cardinal"
)

type JoinInput struct {
	Ok bool
}

type JoinOutput struct {
	Success bool
}

var JoinMsg = cardinal.NewMessageType[JoinInput, JoinOutput]("join",
	cardinal.WithMsgEVMSupport[JoinInput, JoinOutput]())
