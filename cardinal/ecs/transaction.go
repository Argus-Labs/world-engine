package ecs

import (
	"errors"
	"fmt"

	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
	"github.com/argus-labs/world-engine/sign"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/invopop/jsonschema"
)

var _ transaction.ITransaction = NewTransactionType[struct{}, struct{}]("")

// TransactionType helps manage adding transactions (aka events) to the world transaction queue. It also assists
// in the using of transactions inside of System functions.
type TransactionType[In, Out any] struct {
	id      transaction.TypeID
	isIDSet bool
	name    string
	evmType *abi.Type
}

// TransactionQueue is a list of transactions that were queued since the start of the
// last game tick.
type TransactionQueue struct {
	queue transaction.TxMap
}

func NewTransactionType[In, Out any](name string) *TransactionType[In, Out] {
	return &TransactionType[In, Out]{
		name: name,
	}
}

func (t *TransactionType[In, Out]) Name() string {
	return t.name
}

func (t *TransactionType[In, Out]) Schema() (in, out *jsonschema.Schema) {
	return jsonschema.Reflect(new(In)), jsonschema.Reflect(new(Out))
}

// DecodeEVMBytes decodes abi encoded solidity structs into Go structs of the same structure.
func (t *TransactionType[In, Out]) DecodeEVMBytes(bz []byte) (any, error) {
	if t.evmType == nil {
		return nil, errors.New("cannot call DecodeEVMBytes without setting via SetEVMType first")
	}
	args := abi.Arguments{{Type: *t.evmType}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}
	if len(unpacked) < 1 {
		return nil, fmt.Errorf("error decoding EVM bytes: no values could be unpacked into the abi type")
	}
	underlying, ok := unpacked[0].(In)
	if !ok {
		return nil, fmt.Errorf("error decoding EVM bytes: cannot cast %T to %T", unpacked[0], new(In))
	}
	return underlying, nil
}

func (t *TransactionType[In, Out]) SetEVMType(at *abi.Type) {
	t.evmType = at
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
func (t *TransactionType[In, Out]) AddToQueue(world *World, data In, sigs ...*sign.SignedPayload) transaction.TxID {
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
	ID    transaction.TxID
	Value In
	Sig   *sign.SignedPayload
}

func (t *TransactionType[In, Out]) AddError(world *World, id transaction.TxID, err error) {
	world.AddTransactionError(id, err)
}

func (t *TransactionType[In, Out]) SetResult(world *World, id transaction.TxID, result Out) {
	world.SetTransactionResult(id, result)
}

func (t *TransactionType[In, Out]) GetReceipt(world *World, id transaction.TxID) (v Out, errs []error, ok bool) {
	iface, errs, ok := world.GetTransactionReceipt(id)
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

// In extracts all the transactions in the transaction queue that match this TransactionType's ID.
func (t *TransactionType[In, Out]) In(tq *TransactionQueue) []TxData[In] {
	var txs []TxData[In]
	for _, tx := range tq.queue[t.ID()] {
		if val, ok := tx.Value.(In); ok {
			txs = append(txs, TxData[In]{
				ID:    tx.ID,
				Value: val,
				Sig:   tx.Sig,
			})
		}
	}
	return txs
}

func (t *TransactionType[In, Out]) Encode(a any) ([]byte, error) {
	return storage.Encode(a)
}

func (t *TransactionType[In, Out]) Decode(bytes []byte) (any, error) {
	return storage.Decode[In](bytes)
}
