package ecs

import (
	"errors"
	"fmt"
	"reflect"

	ethereumAbi "github.com/ethereum/go-ethereum/accounts/abi"
	"pkg.world.dev/world-engine/cardinal/ecs/abi"
	"pkg.world.dev/world-engine/cardinal/ecs/codec"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

var (
	ErrEVMTypeNotSet = errors.New("EVM type is not set")
)

var _ transaction.ITransaction = &TransactionType[struct{}, struct{}]{}

// TransactionType helps manage adding transactions (aka events) to the world transaction queue. It also assists
// in the using of transactions inside of System functions.
type TransactionType[In, Out any] struct {
	id         transaction.TypeID
	isIDSet    bool
	name       string
	inEVMType  *ethereumAbi.Type
	outEVMType *ethereumAbi.Type
}

func WithTxEVMSupport[In, Out any]() func(transactionType *TransactionType[In, Out]) {
	return func(txt *TransactionType[In, Out]) {
		var in In
		var err error
		txt.inEVMType, err = abi.GenerateABIType(in)
		if err != nil {
			panic(err)
		}

		var out Out
		txt.outEVMType, err = abi.GenerateABIType(out)
		if err != nil {
			panic(err)
		}
	}
}

func NewTransactionType[In, Out any](
	name string,
	opts ...func() func(*TransactionType[In, Out]),
) *TransactionType[In, Out] {
	if name == "" {
		panic("cannot create transaction without name")
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
		panic(fmt.Sprintf("Invalid TransactionType: %s: The In and Out must be both structs", name))
	}

	txt := &TransactionType[In, Out]{
		name: name,
	}
	for _, opt := range opts {
		opt()(txt)
	}
	return txt
}

func (t *TransactionType[In, Out]) Name() string {
	return t.name
}

func (t *TransactionType[In, Out]) IsEVMCompatible() bool {
	return t.inEVMType != nil && t.outEVMType != nil
}

func (t *TransactionType[In, Out]) ID() transaction.TypeID {
	if !t.isIDSet {
		panic(fmt.Sprintf("id on %v is not set", t))
	}
	return t.id
}

var emptySignature = &sign.SignedPayload{}

// AddToQueue adds a transaction with the given data to the world object. The transaction will be executed
// at the next game tick. An optional sign.SignedPayload can be associated with this transaction.
func (t *TransactionType[In, Out]) AddToQueue(world *World, data In, sigs ...*sign.SignedPayload) transaction.TxHash {
	sig := emptySignature
	if len(sigs) > 0 {
		sig = sigs[0]
	}
	_, id := world.AddTransaction(t.ID(), data, sig)
	return id
}

func (t *TransactionType[In, Out]) SetID(id transaction.TypeID) error {
	if t.isIDSet {
		// In games implemented with Cardinal, transactions will only be initialized one time (on startup).
		// In tests, it's often useful to use the same transaction in multiple worlds. This check will allow for the
		// re-initialization of transactions, as long as the ID doesn't change.
		if id == t.id {
			return nil
		}
		return fmt.Errorf("id on transaction %v is already set to %v and cannot change to %d", t, t.id, id)
	}
	t.id = id
	t.isIDSet = true
	return nil
}

type TxData[In any] struct {
	TxHash transaction.TxHash
	Value  In
	Sig    *sign.SignedPayload
}

func (t *TransactionType[In, Out]) AddError(wCtx WorldContext, hash transaction.TxHash, err error) {
	wCtx.GetWorld().AddTransactionError(hash, err)
}

func (t *TransactionType[In, Out]) SetResult(wCtx WorldContext, hash transaction.TxHash, result Out) {
	wCtx.GetWorld().SetTransactionResult(hash, result)
}

func (t *TransactionType[In, Out]) GetReceipt(wCtx WorldContext, hash transaction.TxHash) (
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

func (t *TransactionType[In, Out]) ForEach(wCtx WorldContext, fn func(TxData[In]) (Out, error)) {
	for _, tx := range t.In(wCtx) {
		if result, err := fn(tx); err != nil {
			wCtx.Logger().Err(err).Msgf("tx %s from %s encountered an error with tx=%+v", tx.TxHash,
				tx.Sig.PersonaTag, tx.Value)
			t.AddError(wCtx, tx.TxHash, err)
		} else {
			t.SetResult(wCtx, tx.TxHash, result)
		}
	}
}

// In extracts all the transactions in the transaction queue that match this TransactionType's ID.
func (t *TransactionType[In, Out]) In(wCtx WorldContext) []TxData[In] {
	tq := wCtx.GetTxQueue()
	var txs []TxData[In]
	for _, tx := range tq.ForID(t.ID()) {
		if val, ok := tx.Value.(In); ok {
			txs = append(txs, TxData[In]{
				TxHash: tx.TxHash,
				Value:  val,
				Sig:    tx.Sig,
			})
		}
	}
	return txs
}

func (t *TransactionType[In, Out]) Encode(a any) ([]byte, error) {
	return codec.Encode(a)
}

func (t *TransactionType[In, Out]) Decode(bytes []byte) (any, error) {
	return codec.Decode[In](bytes)
}

// ABIEncode encodes the input to the transactions matching evm type. If the input is not either of the transactions
// evm types, an error is returned.
func (t *TransactionType[In, Out]) ABIEncode(v any) ([]byte, error) {
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

// DecodeEVMBytes decodes abi encoded solidity structs into the transaction's "In" type.
func (t *TransactionType[In, Out]) DecodeEVMBytes(bz []byte) (any, error) {
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
