package msg

import (
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/testutils"
)

type JoinInput struct {
	Ok bool
}

type JoinOutput struct {
	Success bool
}

var JoinMsg = testutils.NewMessageType[JoinInput, JoinOutput]("join",
	message.WithMsgEVMSupport[JoinInput, JoinOutput]())
