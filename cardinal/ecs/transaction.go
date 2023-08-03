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

var _ transaction.ITransaction = NewTransactionType[struct{}]("")

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
	ids        map[transaction.TypeID][]TxID
	queue      map[transaction.TypeID][]any
	signatures map[transaction.TypeID][]*sign.SignedPayload
}

func NewTransactionType[In, Out any](name string) *TransactionType[In, Out] {
	return &TransactionType[In, Out]{
		name: name,
	}
}

func (t *TransactionType[In, Out]) Name() string {
	return t.name
}

func (t *TransactionType[In, Out]) Schema() *jsonschema.Schema {
	return jsonschema.Reflect(new(T))
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

func (t *TransactionType[In, Out]) GetResult(world *World, id TxID) (r Out, errs []error, ok bool) {
	iface, errs, ok := world.GetTransactionResult(id)
	if !ok {
		return nil, nil, false
	}
	val, ok :=

	r, ok = v.(Out)
	return r, ok
}

// AddToQueue adds a transaction with the given data to the world object. The transaction will be executed
// at the next game tick. An optional sign.SignedPayload can be associated with this transaction.
func (t *TransactionType[In, Out]) AddToQueue(world *World, data In, sigs ...*sign.SignedPayload) TxID {
	var sig *sign.SignedPayload
	if len(sigs) > 0 {
		sig = sigs[0]
	}
	return world.AddTransaction(t.ID(), data, sig)
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

type TxData[T any] struct {
	ID  TxID
	Val T
	Sig *sign.SignedPayload
}

func (t *TransactionType[In, Out]) AddError(world *World, id TxID, err error) {
	panic("not implemeted")
}
func (t *TransactionType[In, Out]) SetResult(world *World, id TxID, result Out) {
	panic("not implemented")
}

func (t *TransactionType[In, Out]) TxsIn(tq *TransactionQueue) []TxData[In] {
	var results []TxData[In]
	for i, tx := range tq.queue[t.ID()] {
		if val, ok := tx.(In); ok {
			id := tq.ids[t.ID()][i]
			sig := tq.signatures[t.ID()][i]
			results = append(results, TxData[In]{
				ID:  id,
				Val: val,
				Sig: sig,
			})
		}
	}
	return results
}

// In extracts all the transactions in the transaction queue that match this TransactionType's ID.
func (t *TransactionType[In, Out]) In(tq *TransactionQueue) []In {
	var txs []In
	for _, tx := range tq.queue[t.ID()] {
		if val, ok := tx.(In); ok {
			txs = append(txs, val)
		}
	}
	return txs
}

// TxsAndSigsIn extracts all the transactions and their related signatures in the transaction queue
// that match this TransactionType's ID.
func (t *TransactionType[In, Out]) TxsAndSigsIn(tq *TransactionQueue) ([]In, []*sign.SignedPayload) {
	var txs []In
	var sigs []*sign.SignedPayload
	for i, tx := range tq.queue[t.ID()] {
		if val, ok := tx.(In); ok {
			txs = append(txs, val)
			sigs = append(sigs, tq.signatures[t.ID()][i])
		}
	}
	return txs, sigs
}

func (t *TransactionType[In, Out]) Encode(a any) ([]byte, error) {
	return storage.Encode(a)
}

func (t *TransactionType[In, Out]) Decode(bytes []byte) (any, error) {
	return storage.Decode[In, Out](bytes)
}
