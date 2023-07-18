package ecs

import (
	"errors"
	"fmt"

	"github.com/argus-labs/world-engine/sign"
	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
)

var _ transaction.ITransaction = NewTransactionType[struct{}]("")

// TransactionType helps manage adding transactions (aka events) to the world transaction queue. It also assists
// in the using of transactions inside of System functions.
type TransactionType[T any] struct {
	id      transaction.TypeID
	isIDSet bool
	name    string
	evmType *abi.Type
}

// TransactionQueue is a list of transactions that were queued since the start of the
// last game tick.
type TransactionQueue struct {
	queue      map[transaction.TypeID][]any
	signatures map[transaction.TypeID][]*sign.SignedPayload
}

func NewTransactionType[T any](name string) *TransactionType[T] {
	return &TransactionType[T]{
		name: name,
	}
}

func (t *TransactionType[T]) Name() string {
	return t.name
}

// DecodeEVMBytes decodes abi encoded solidity structs into Go structs of the same structure.
func (t *TransactionType[T]) DecodeEVMBytes(bz []byte) (any, error) {
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
	underlying, ok := unpacked[0].(T)
	if !ok {
		return nil, fmt.Errorf("error decoding EVM bytes: cannot cast %T to %T", unpacked[0], new(T))
	}
	return underlying, nil
}

func (t *TransactionType[T]) SetEVMType(at *abi.Type) {
	t.evmType = at
}

func (t *TransactionType[T]) ID() transaction.TypeID {
	if !t.isIDSet {
		panic(fmt.Sprintf("id on %v is not set", t))
	}
	return t.id
}

// AddToQueue adds a transaction with the given data to the world object. The transaction will be executed
// at the next game tick. An optional sign.SignedPayload can be associated with this transaction.
func (t *TransactionType[T]) AddToQueue(world *World, data T, sigs ...*sign.SignedPayload) {
	var sig *sign.SignedPayload
	if len(sigs) > 0 {
		sig = sigs[0]
	}
	world.AddTransaction(t.ID(), data, sig)
}

func (t *TransactionType[T]) SetID(id transaction.TypeID) error {
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

// In extracts all the transactions in the transaction queue that match this TransactionType's ID.
func (t *TransactionType[T]) In(tq *TransactionQueue) []T {
	var txs []T
	for _, tx := range tq.queue[t.ID()] {
		if val, ok := tx.(T); ok {
			txs = append(txs, val)
		}
	}
	return txs
}

// TxAndSigsIn extracts all the transactions and their related signatures in the transaction queue 
// that match this TransactionType's ID.
func (t *TransactionType[T]) TxsAndSigsIn(tq *TransactionQueue) ([]T, []*sign.SignedPayload) {
	var txs []T
	var sigs []*sign.SignedPayload
	for i, tx := range tq.queue[t.ID()] {
		if val, ok := tx.(T); ok {
			txs = append(txs, val)
			sigs = append(sigs, tq.signatures[t.ID()][i])
		}
	}
	return txs, sigs
}

func (t *TransactionType[T]) Encode(a any) ([]byte, error) {
	return storage.Encode(a)
}

func (t *TransactionType[T]) Decode(bytes []byte) (any, error) {
	return storage.Decode[T](bytes)
}
