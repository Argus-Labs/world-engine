package message

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/abi"
	"pkg.world.dev/world-engine/cardinal/codec"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"

	ethereumAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrEVMTypeNotSet = errors.New("EVM type is not set")
)

var _ types.Message = &MessageType[struct{}, struct{}]{}

// MessageType manages a user defined state transition message struct.
type MessageType[In, Out any] struct { //nolint:revive // this is fine for now.
	id         types.MessageID
	isIDSet    bool
	name       string
	group      string
	inEVMType  *ethereumAbi.Type
	outEVMType *ethereumAbi.Type
}

func isStruct[T any]() bool {
	var in T
	inType := reflect.TypeOf(in)
	inKind := inType.Kind()
	return (inKind == reflect.Pointer &&
		inType.Elem().Kind() == reflect.Struct) ||
		inKind == reflect.Struct
}

// NewMessageType creates a new message type. It accepts two generic type parameters: the first for the message input,
// which defines the data needed to make a state transition, and the second for the message output, commonly used
// for the results of a state transition.
func NewMessageType[In, Out any](
	name string,
	opts ...MessageOption[In, Out],
) *MessageType[In, Out] {
	if name == "" {
		panic("cannot create message without name")
	}
	if !isStruct[In]() || !isStruct[Out]() {
		panic(fmt.Sprintf("Invalid MessageType: %s: The In and Out must be both structs", name))
	}
	msg := &MessageType[In, Out]{
		name:  name,
		group: "game",
	}
	for _, opt := range opts {
		opt(msg)
	}
	return msg
}

func (t *MessageType[In, Out]) SetName(name string) {
	t.name = name
}

func (t *MessageType[In, Out]) SetGroup(group string) {
	t.group = group
}

func (t *MessageType[In, Out]) Name() string {
	return t.name
}

func (t *MessageType[In, Out]) Group() string {
	return t.group
}

func (t *MessageType[In, Out]) IsEVMCompatible() bool {
	return t.inEVMType != nil && t.outEVMType != nil
}

func (t *MessageType[In, Out]) ID() types.MessageID {
	if !t.isIDSet {
		panic(fmt.Sprintf("id on msg %q is not set", t.name))
	}
	return t.id
}

func (t *MessageType[In, Out]) SetID(id types.MessageID) error {
	if t.isIDSet {
		// In games implemented with Cardinal, messages will only be initialized one time (on startup).
		// In tests, it's often useful to use the same message in multiple engines. This check will allow for the
		// re-initialization of messages, as long as the EntityID doesn't change.
		if id == t.id {
			return nil
		}
		return eris.Errorf("id on message %q is already set to %d and cannot change to %d", t.name, t.id, id)
	}
	t.id = id
	t.isIDSet = true
	return nil
}

type TxData[In any] struct {
	Hash types.TxHash
	Msg  In
	Tx   *sign.Transaction
}

func (t *MessageType[In, Out]) AddError(wCtx engine.Context, hash types.TxHash, err error) {
	wCtx.AddMessageError(hash, err)
}

func (t *MessageType[In, Out]) SetResult(wCtx engine.Context, hash types.TxHash, result Out) {
	wCtx.SetMessageResult(hash, result)
}

func (t *MessageType[In, Out]) GetReceipt(wCtx engine.Context, hash types.TxHash) (
	v Out, errs []error, ok bool,
) {
	iface, errs, ok := wCtx.GetTransactionReceipt(hash)
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

func (t *MessageType[In, Out]) Each(wCtx engine.Context, fn func(TxData[In]) (Out, error)) {
	for _, txData := range t.In(wCtx) {
		if result, err := fn(txData); err != nil {
			err = eris.Wrap(err, "")
			wCtx.Logger().Err(err).Msgf("tx %s from %s encountered an error with message=%+v and stack trace:\n %s",
				txData.Hash,
				txData.Tx.PersonaTag,
				txData.Msg,
				eris.ToString(err, true),
			)
			t.AddError(wCtx, txData.Hash, err)
		} else {
			t.SetResult(wCtx, txData.Hash, result)
		}
	}
}

// In extracts all the TxData in the tx pool that match this MessageType's ID.
func (t *MessageType[In, Out]) In(wCtx engine.Context) []TxData[In] {
	tq := wCtx.GetTxPool()
	var txs []TxData[In]
	for _, txData := range tq.ForID(t.ID()) {
		if val, ok := txData.Msg.(In); ok {
			txs = append(txs, TxData[In]{
				Hash: txData.TxHash,
				Msg:  val,
				Tx:   txData.Tx,
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
		return nil, eris.Wrap(ErrEVMTypeNotSet, "")
	}

	var args ethereumAbi.Arguments
	var input any
	//nolint:gocritic // it's fine.
	switch in := v.(type) {
	case Out:
		input = in
		args = ethereumAbi.Arguments{{Type: *t.outEVMType}}
	case In:
		input = in
		args = ethereumAbi.Arguments{{Type: *t.inEVMType}}
	default:
		return nil, eris.Errorf("expected input to be of type %T or %T, got %T", new(In), new(Out), v)
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
	err = eris.Wrap(err, "")
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, eris.Errorf("error decoding EVM bytes: no values could be unpacked into the abi type")
	}
	input, err := abi.SerdeInto[In](unpacked[0])
	if err != nil {
		return nil, err
	}
	return input, nil
}

// GetInFieldInformation returns a map of the fields of the message's "In" type and it's field types.
func (t *MessageType[In, Out]) GetInFieldInformation() map[string]any {
	return types.GetFieldInformation(reflect.TypeOf(new(In)).Elem())
}

// -------------------------- Options --------------------------

type MessageOption[In, Out any] func(mt *MessageType[In, Out]) //nolint:revive // this is fine for now

func WithMsgEVMSupport[In, Out any]() MessageOption[In, Out] {
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

// WithCustomMessageGroup sets a custom group for the message.
// By default, messages are registered under the "game" group which maps it to the /tx/game/:txType route.
// This option allows you to set a custom group, which allow you to register the message
// under /tx/<custom_group>/:txType.
func WithCustomMessageGroup[In, Out any](group string) MessageOption[In, Out] {
	return func(mt *MessageType[In, Out]) {
		mt.group = group
	}
}
