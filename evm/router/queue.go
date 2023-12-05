package router

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog/log"
	v1 "pkg.world.dev/world-engine/rift/router/v1"
	"sync"
)

type gameShardMsg struct {
	// the message to send to the game shard.
	msg *v1.SendMessageRequest
	// the namespace of the game shard.
	namespace string
}

func (m *msgQueue) Set(sender common.Address, namespace string, msg *v1.SendMessageRequest) error {
	m.mut.Lock()
	defer m.mut.Unlock()
	m.queue[sender] = &gameShardMsg{msg, namespace}
	log.Logger.Debug().Msgf("queued message to %q", namespace)
	return nil
}

func (m *msgQueue) Message(sender common.Address) (*gameShardMsg, bool) {
	m.mut.Lock()
	defer m.mut.Unlock()
	msg, ok := m.queue[sender]
	return msg, ok
}

func (m *msgQueue) Remove(sender common.Address) {
	m.mut.Lock()
	defer m.mut.Unlock()
	delete(m.queue, sender)
}

func (m *msgQueue) IsSet(address common.Address) bool {
	m.mut.Lock()
	defer m.mut.Unlock()
	_, isSet := m.queue[address]
	return isSet
}

func (m *msgQueue) Clear() {
	m.mut.Lock()
	defer m.mut.Unlock()
	clear(m.queue)
}

type msgQueue struct {
	mut   sync.Mutex
	queue map[common.Address]*gameShardMsg
}

func newMsgQueue() *msgQueue {
	return &msgQueue{
		mut:   sync.Mutex{},
		queue: make(map[common.Address]*gameShardMsg),
	}
}
