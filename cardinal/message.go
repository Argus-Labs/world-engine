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

// MessageType represents a type of message that can be executed on the World object. The Input struct represents
// the input, and the Result struct represents the result of processing the message.
type MessageType[Input, Result any] struct {
	impl *ecs.MessageType[Input, Result]
}

// NewMessageType creates a new instance of a MessageType.
func NewMessageType[Input, Result any](name string) *MessageType[Input, Result] {
	return &MessageType[Input, Result]{
		impl: ecs.NewMessageType[Input, Result](name),
	}
}

// NewMessageTypeWithEVMSupport creates a new instance of a MessageType, with EVM messages enabled.
// This allows this message to be sent from EVM smart contracts on the EVM base shard.
func NewMessageTypeWithEVMSupport[Input, Result any](name string) *MessageType[Input, Result] {
	return &MessageType[Input, Result]{
		impl: ecs.NewMessageType[Input, Result](name, ecs.WithMsgEVMSupport[Input, Result]),
	}
}

// AddToQueue is not meant to be used in production whatsoever, it is exposed here for usage in tests.
func (t *MessageType[Input, Result]) AddToQueue(world *World, data Input, sigs ...*sign.Transaction) TxHash {
	txHash := t.impl.AddToQueue(world.instance, data, sigs...)
	return txHash
}

// AddError adds the given error to the transaction identified by the given hash. Multiple errors can be
// added to the same message hash.
func (t *MessageType[Input, Result]) AddError(wCtx WorldContext, hash TxHash, err error) {
	t.impl.AddError(wCtx.Instance(), hash, err)
}

// SetResult sets the result of the message identified by the given hash. Only one result may be associated
// with a message hash, so calling this multiple times will clobber previously set results.
func (t *MessageType[Input, Result]) SetResult(wCtx WorldContext, hash TxHash, result Result) {
	t.impl.SetResult(wCtx.Instance(), hash, result)
}

// GetReceipt returns the result (if any) and errors (if any) associated with the given hash. If false is returned,
// the hash is not recognized, so the returned result and errors will be empty.
func (t *MessageType[Input, Result]) GetReceipt(wCtx WorldContext, hash TxHash) (Result, []error, bool) {
	return t.impl.GetReceipt(wCtx.Instance(), hash)
}

func (t *MessageType[Input, Result]) ForEach(wCtx WorldContext, fn func(TxData[Input]) (Result, error)) {
	adapterFn := func(ecsTxData ecs.TxData[Input]) (Result, error) {
		adaptedTx := TxData[Input]{impl: ecsTxData}
		return fn(adaptedTx)
	}
	t.impl.ForEach(wCtx.Instance(), adapterFn)
}

// In returns the TxData in the given transaction queue that match this message's type.
func (t *MessageType[Input, Result]) In(wCtx WorldContext) []TxData[Input] {
	ecsTxData := t.impl.In(wCtx.Instance())
	out := make([]TxData[Input], 0, len(ecsTxData))
	for _, tx := range ecsTxData {
		out = append(out, TxData[Input]{
			impl: tx,
		})
	}
	return out
}

// Convert implements the AnyMessageType interface which allows a MessageType to be registered
// with a World via RegisterMessages.
func (t *MessageType[Input, Result]) Convert() message.Message {
	return t.impl
}

// Hash returns the hash of a specific message, which is used to associated results and errors with a specific
// message.
func (t *TxData[T]) Hash() TxHash {
	return t.impl.Hash
}

// Msg returns the input value of a message.
func (t *TxData[T]) Msg() T {
	return t.impl.Msg
}

// Tx returns the transaction data.
func (t *TxData[T]) Tx() *sign.Transaction {
	return t.impl.Tx
}
