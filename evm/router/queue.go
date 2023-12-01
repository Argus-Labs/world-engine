package router

import (
	"errors"
	"github.com/ethereum/go-ethereum/common"
	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	v1 "pkg.world.dev/world-engine/rift/router/v1"
)

type gameShardMsg struct {
	// the message to send to the game shard.
	msg *v1.SendMessageRequest
	// the namespace of the game shard.
	namespace string
}

var (
	ErrAlreadySet = errors.New("queue is already set for this address. only one cross-shard message may be queued per EVM block")
)

func (m *msgQueue) Set(sender common.Address, namespace string, msg *v1.SendMessageRequest) error {
	log.Logger.Info().Msg("inside Set")
	if m.IsSet(sender) {
		log.Logger.Error().Msg("error: queue already set for this sender. only one cross-shard msg may be queued per-block.")
		return eris.Wrap(ErrAlreadySet, sender.String())
	}
	m.queue[sender] = &gameShardMsg{msg, namespace}
	log.Logger.Debug().Msg("in queue: msg queued")
	return nil
}

func (m *msgQueue) Message(sender common.Address) (*gameShardMsg, bool) {
	msg, ok := m.queue[sender]
	return msg, ok
}

func (m *msgQueue) Remove(sender common.Address) {
	delete(m.queue, sender)
}

func (m *msgQueue) IsSet(address common.Address) bool {
	log.Logger.Info().Msg("inside IsSet")
	_, isSet := m.queue[address]
	log.Logger.Info().Msg("returning from IsSet")
	return isSet
}

func (m *msgQueue) Clear() {
	clear(m.queue)
}

type msgQueue struct {
	queue map[common.Address]*gameShardMsg
}

func newMsgQueue() *msgQueue {
	return &msgQueue{
		queue: make(map[common.Address]*gameShardMsg),
	}
}
