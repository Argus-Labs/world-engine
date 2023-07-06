package ecs

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"

	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	"github.com/argus-labs/world-engine/cardinal/ecs/transaction"
)

var _ transaction.ITransaction = NewTransactionType[struct{}]()

// TransactionType helps manage adding transactions (aka events) to the world transaction queue. It also assists
// in the using of transactions inside of System functions.
type TransactionType[T any] struct {
	id      transaction.TypeID
	isIDSet bool
	evmType abi.Type
}

// TransactionQueue is a list of transactions that were queued since the start of the
// last game tick.
type TransactionQueue struct {
	queue map[transaction.TypeID][]any
}

func NewTransactionType[T any]() *TransactionType[T] {
	return &TransactionType[T]{}
}

// DecodeEVMBytes decodes abi encoded solidity structs into Go structs of the same structure.
func (t *TransactionType[T]) DecodeEVMBytes(bz []byte) (any, error) {
	args := abi.Arguments{{Type: t.evmType}}
	unpacked, err := args.Unpack(bz)
	if err != nil {
		return nil, err
	}

	underlying, ok := unpacked[0].(T)
	if !ok {
		return nil, fmt.Errorf("error decoding EVM bytes: cannot cast %T to %T", unpacked[0], new(T))
	}
	return underlying, nil
}

func (t *TransactionType[T]) SetEVMType(at abi.Type) {
	t.evmType = at
}

func (t *TransactionType[T]) ID() transaction.TypeID {
	if !t.isIDSet {
		panic(fmt.Sprintf("id on %v is not set", t))
	}
	return t.id
}

// AddToQueue adds a transaction with the given data to the world object. The transaction will be executed
// at the next game tick.
func (t *TransactionType[T]) AddToQueue(world *World, data T) {
	world.addTransaction(t.ID(), data)
}

func (t *TransactionType[T]) SetID(id transaction.TypeID) error {
	if t.isIDSet {
		return fmt.Errorf("id on transaction %v is already set to %v and cannot change to %d", t, t.id, id)
	}
	t.id = id
	t.isIDSet = true
	return nil
}

// In extracts all the transactions from the transaction queue that match this TransactionType's name.
func (t *TransactionType[T]) In(tq *TransactionQueue) []T {
	var txs []T
	for _, tx := range tq.queue[t.ID()] {
		if val, ok := tx.(T); ok {
			txs = append(txs, val)
		}
	}
	return txs
}

func (t *TransactionType[T]) Encode(a any) ([]byte, error) {
	return storage.Encode(a)
}

func (t *TransactionType[T]) Decode(bytes []byte) (any, error) {
	return storage.Decode[T](bytes)
}
