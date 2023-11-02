package router

import (
	v1 "pkg.world.dev/world-engine/rift/router/v1"
)

type msgQueue struct {
	// the message to send to the game shard.
	msg *v1.SendMessageRequest
	// the namespace of the game shard.
	namespace string
}

func (m *msgQueue) Set(namespace string, msg *v1.SendMessageRequest) {
	m.msg = msg
	m.namespace = namespace
}

func (m *msgQueue) IsSet() bool {
	return m.msg != nil && m.namespace != ""
}

func (m *msgQueue) Clear() {
	m.msg = nil
	m.namespace = ""
}
