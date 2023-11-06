package ecs

import (
	"errors"
	"fmt"
	"pkg.world.dev/world-engine/cardinal/ecs/message"
	"reflect"

	ethereumAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"pkg.world.dev/world-engine/cardinal/ecs/abi"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrEVMTypeNotSet = errors.New("EVM type is not set")
)

var _ message.Message = &MessageType[struct{}, struct{}]{}

// MessageType manages a user defined state transition message struct.
type MessageType[In, Out any] struct {
	id         message.TypeID
	isIDSet    bool
	name       string
	inEVMType  *ethereumAbi.Type
	outEVMType *ethereumAbi.Type
}

func WithMsgEVMSupport[In, Out any]() func(messageType *MessageType[In, Out]) {
	return func(msg *MessageType[In, Out]) {
		var in In
		var err error
		msg.inEVMType, err = abi.GenerateABIType(in)
		if err != nil {
			panic(err)
		}

		var out Out
		msg.outEVMType, err = abi.GenerateABIType(out)
		if err != nil {
			panic(err)
		}
	}
}

// NewMessageType creates a new message type. It accepts two generic type parameters: the first for the message input,
// which defines the data needed to make a state transition, and the second for the message output, commonly used
// for the results of a state transition.
func NewMessageType[In, Out any](
	name string,
	opts ...func() func(*MessageType[In, Out]),
) *MessageType[In, Out] {
	if name == "" {
		panic("cannot create message without name")
	}
	var in In
	var out Out
	inType := reflect.TypeOf(in)
	inKind := inType.Kind()
	inValid := false
	if (inKind == reflect.Pointer && inType.Elem().Kind() == reflect.Struct) || inKind == reflect.Struct {
		inValid = true
	}
	outType := reflect.TypeOf(out)
	outKind := inType.Kind()
	outValid := false
	if (outKind == reflect.Pointer && outType.Elem().Kind() == reflect.Struct) || outKind == reflect.Struct {
		outValid = true
	}

	if !outValid || !inValid {
		panic(fmt.Sprintf("Invalid MessageType: %s: The In and Out must be both structs", name))
	}

	msg := &MessageType[In, Out]{
		name: name,
	}
	for _, opt := range opts {
		opt()(msg)
	}
	return msg
}

func (t *MessageType[In, Out]) Name() string {
	return t.name
}

func (t *MessageType[In, Out]) IsEVMCompatible() bool {
	return t.inEVMType != nil && t.outEVMType != nil
}

func (t *MessageType[In, Out]) ID() message.TypeID {
	if !t.isIDSet {
		panic(fmt.Sprintf("id on msg %q is not set", t.name))
	}
	return t.id
}

var emptyTx = &sign.Transaction{}

// AddToQueue adds a message with the given data to the world. The message will be executed
// at the next game tick. An optional sign.Transaction can be associated with this message.
func (t *MessageType[In, Out]) AddToQueue(world *World, data In, sigs ...*sign.Transaction) message.Hash {
	sig := emptyTx
	if len(sigs) > 0 {
		sig = sigs[0]
	}
	_, id := world.AddTransaction(t.ID(), data, sig)
	return id
}

func (t *MessageType[In, Out]) SetID(id message.TypeID) error {
	if t.isIDSet {
		// In games implemented with Cardinal, messages will only be initialized one time (on startup).
		// In tests, it's often useful to use the same message in multiple worlds. This check will allow for the
		// re-initialization of messages, as long as the ID doesn't change.
		if id == t.id {
			return nil
		}
		return fmt.Errorf("id on message %q is already set to %d and cannot change to %d", t.name, t.id, id)
	}
	t.id = id
	t.isIDSet = true
	return nil
}

type TxData[In any] struct {
	MsgHash message.Hash
	Msg     In
	Tx      *sign.Transaction
}

func (t *MessageType[In, Out]) AddError(wCtx WorldContext, hash message.Hash, err error) {
	wCtx.GetWorld().AddMessageError(hash, err)
}

func (t *MessageType[In, Out]) SetResult(wCtx WorldContext, hash message.Hash, result Out) {
	wCtx.GetWorld().SetMessageResult(hash, result)
}

func (t *MessageType[In, Out]) GetReceipt(wCtx WorldContext, hash message.Hash) (
	v Out, errs []error, ok bool,
) {
	world := wCtx.GetWorld()
	iface, errs, ok := world.GetTransactionReceipt(hash)
	if !ok {
		return v, nil, false
	}
	// if iface is nil, maybe the result has never been set. The errors may still be valid.
	if iface == nil {
		return v, errs, true
	}
	value, ok := iface.(Out)
	if !ok {
		return v, nil, false
	}
	return value, errs, true
}

func (t *MessageType[In, Out]) ForEach(wCtx WorldContext, fn func(TxData[In]) (Out, error)) {
	for _, txData := range t.In(wCtx) {
		if result, err := fn(txData); err != nil {
			wCtx.Logger().Err(err).Msgf("tx %s from %s encountered an error with message=%+v",
				txData.MsgHash,
				txData.Tx.PersonaTag,
				txData.Msg,
			)
			t.AddError(wCtx, txData.MsgHash, err)
		} else {
			t.SetResult(wCtx, txData.MsgHash, result)
		}
	}
}

// In extracts all the TxData in the tx queue that match this MessageType's ID.
func (t *MessageType[In, Out]) In(wCtx WorldContext) []TxData[In] {
	tq := wCtx.GetTxQueue()
	var txs []TxData[In]
	for _, txData := range tq.ForID(t.ID()) {
		if val, ok := txData.Msg.(In); ok {
			txs = append(txs, TxData[In]{
				MsgHash: txData.MsgHash,
				Msg:     val,
				Tx:      txData.Tx,
			})
		}
	}
	return txs
}

func (t *MessageType[In, Out]) Encode(a any) ([]byte, error) {
	return codec.Encode(a)
}

func (t *MessageType[In, Out]) Decode(bytes []byte) (any, error) {
	return codec.Decode[In](bytes)
}

// ABIEncode encodes the input to the message's matching evm type. If the input is not either of the message's
// evm types, an error is returned.
func (t *MessageType[In, Out]) ABIEncode(v any) ([]byte, error) {
	if t.inEVMType == nil || t.outEVMType == nil {
		return nil, ErrEVMTypeNotSet
	}

	var args ethereumAbi.Arguments
	var input any
	//nolint:gocritic // its fine.
	switch in := v.(type) {
	case Out:
		input = in
		args = ethereumAbi.Arguments{{Type: *t.outEVMType}}
	case In:
		input = in
		args = ethereumAbi.Arguments{{Type: *t.inEVMType}}
	default:
		return nil, fmt.Errorf("expected input to be of type %T or %T, got %T", new(In), new(Out), v)
	}

	return args.Pack(input)
}

// DecodeEVMBytes decodes abi encoded solidity structs into the message's "In" type.
func (t *MessageType[In, Out]) DecodeEVMBytes(bz []byte) (any, error) {
	if t.inEVMType == nil {
		return nil, ErrEVMTypeNotSet
	}
	args := ethereumAbi.Arguments{{Type: *t.inEVMType}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, fmt.Errorf("error decoding EVM bytes: no values could be unpacked into the abi type")
	}
	input, err := abi.SerdeInto[In](unpacked[0])
	if err != nil {
		return nil, err
	}
	return input, nil
}
