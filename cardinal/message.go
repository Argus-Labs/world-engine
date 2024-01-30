package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
)

// NewMessageType creates a new instance of a MessageType.
func NewMessageType[Input, Result any](name string) *ecs.MessageType[Input, Result] {
	return ecs.NewMessageType[Input, Result](name)
}

// NewMessageTypeWithEVMSupport creates a new instance of a MessageType, with EVM messages enabled.
// This allows this message to be sent from EVM smart contracts on the EVM base shard.
func NewMessageTypeWithEVMSupport[Input, Result any](name string) *ecs.MessageType[Input, Result] {
	return ecs.NewMessageType[Input, Result](name, ecs.WithMsgEVMSupport[Input, Result]())
}
