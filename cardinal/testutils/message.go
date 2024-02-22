package testutils

import (
	"pkg.world.dev/world-engine/cardinal/message"
)

type FooMessage struct {
	Bar string
}

type FooResponse struct {
}

var FooTx = message.NewMessageType[FooMessage, FooResponse](
	"foo",
	message.WithMsgEVMSupport[FooMessage, FooResponse](),
)
