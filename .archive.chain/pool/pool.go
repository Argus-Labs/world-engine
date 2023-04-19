package pool

import (
	"sync"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type MsgPoolSender interface {
	Send(...sdk.Msg)
}

type MsgPoolReceiver interface {
	Drain() []sdk.Msg
}

type Response struct {
	Result sdk.Result
	Err    error
}

var (
	_ MsgPoolSender   = &MsgPool{}
	_ MsgPoolReceiver = &MsgPool{}
)

type MsgPool struct {
	queue []sdk.Msg
	lock  sync.RWMutex
}

func (m *MsgPool) Drain() []sdk.Msg {
	m.lock.Lock()
	queue := m.queue
	m.queue = make([]sdk.Msg, 0, cap(queue))
	m.lock.Unlock()
	return queue
}

func (m *MsgPool) Send(msgs ...sdk.Msg) {
	m.lock.Lock()
	m.queue = append(m.queue, msgs...)
	m.lock.Unlock()
}

// NewMsgPool returns a new MsgPool. initialBufferSize is the initial amount of cap space to provide to the msg slice.
func NewMsgPool(initialBufferSize int) *MsgPool {
	mp := &MsgPool{
		queue: make([]sdk.Msg, 0, initialBufferSize),
		lock:  sync.RWMutex{},
	}
	return mp
}
