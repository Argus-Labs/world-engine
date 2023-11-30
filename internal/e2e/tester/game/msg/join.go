package msg

import (
	"pkg.world.dev/world-engine/cardinal"
)

type JoinInput struct {
	Ok bool
}

type JoinOutput struct{}

var JoinMsg = cardinal.NewMessageTypeWithEVMSupport[JoinInput, JoinOutput]("join")
