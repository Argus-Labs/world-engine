package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/message"
	"pkg.world.dev/world-engine/sign"
)

// AnyMessage is implemented by the return value of NewMessageType and is used in RegisterMessages; any
// message created by NewMessageType can be registered with a World object via RegisterMessages.
type AnyMessage interface {
	Convert() message.Message
}

// TxData represents a single transaction.
type TxData[T any] struct {
	impl ecs.TxData[T]
}

// MessageType represents a type of message that can be executed on the World object. The Msg struct represents
// the input for a specific transaction, and the Result struct represents the result of processing the transaction.
type MessageType[Msg, Result any] struct {
	impl *ecs.MessageType[Msg, Result]
}

// NewMessageType creates a new instance of a MessageType.
func NewMessageType[Msg, Result any](name string) *MessageType[Msg, Result] {
	return &MessageType[Msg, Result]{
		impl: ecs.NewMessageType[Msg, Result](name),
	}
}

// NewMessageTypeWithEVMSupport creates a new instance of a MessageType, with EVM transactions enabled.
// This allows this transaction to be sent from EVM smart contracts on the EVM base shard.
func NewMessageTypeWithEVMSupport[Msg, Result any](name string) *MessageType[Msg, Result] {
	return &MessageType[Msg, Result]{
		impl: ecs.NewMessageType[Msg, Result](name, ecs.WithMsgEVMSupport[Msg, Result]),
	}
}

// AddToQueue is not meant to be used in production whatsoever, it is exposed here for usage in tests.
func (t *MessageType[Msg, Result]) AddToQueue(world *World, data Msg, sigs ...*sign.Transaction) MsgHash {
	txHash := t.impl.AddToQueue(world.implWorld, data, sigs...)
	return txHash
}

// AddError adds the given error to the transaction identified by the given hash. Multiple errors can be
// added to the same message hash.
func (t *MessageType[Msg, Result]) AddError(wCtx WorldContext, hash MsgHash, err error) {
	t.impl.AddError(wCtx.getECSWorldContext(), hash, err)
}

// SetResult sets the result of the message identified by the given hash. Only one result may be associated
// with a message hash, so calling this multiple times will clobber previously set results.
func (t *MessageType[Msg, Result]) SetResult(wCtx WorldContext, hash MsgHash, result Result) {
	t.impl.SetResult(wCtx.getECSWorldContext(), hash, result)
}

// GetReceipt returns the result (if any) and errors (if any) associated with the given hash. If false is returned,
// the hash is not recognized, so the returned result and errors will be empty.
func (t *MessageType[Msg, Result]) GetReceipt(wCtx WorldContext, hash MsgHash) (Result, []error, bool) {
	return t.impl.GetReceipt(wCtx.getECSWorldContext(), hash)
}

func (t *MessageType[Msg, Result]) ForEach(wCtx WorldContext, fn func(TxData[Msg]) (Result, error)) {
	adapterFn := func(ecsTxData ecs.TxData[Msg]) (Result, error) {
		adaptedTx := TxData[Msg]{impl: ecsTxData}
		return fn(adaptedTx)
	}
	t.impl.ForEach(wCtx.getECSWorldContext(), adapterFn)
}

// In returns the TxData in the given transaction queue that match this message's type.
func (t *MessageType[Msg, Result]) In(wCtx WorldContext) []TxData[Msg] {
	ecsTxData := t.impl.In(wCtx.getECSWorldContext())
	out := make([]TxData[Msg], 0, len(ecsTxData))
	for _, tx := range ecsTxData {
		out = append(out, TxData[Msg]{
			impl: tx,
		})
	}
	return out
}

// Convert implements the AnyMessageType interface which allows a MessageType to be registered
// with a World via RegisterMessages.
func (t *MessageType[Msg, Result]) Convert() message.Message {
	return t.impl
}

// Hash returns the hash of a specific message, which is used to associated results and errors with a specific
// message.
func (t *TxData[T]) Hash() MsgHash {
	return t.impl.MsgHash
}

// Msg returns the input value of a message.
func (t *TxData[T]) Msg() T {
	return t.impl.Msg
}

// Tx returns the transaction data.
func (t *TxData[T]) Tx() *sign.Transaction {
	return t.impl.Tx
}
