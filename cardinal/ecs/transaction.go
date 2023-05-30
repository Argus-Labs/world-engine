package ecs

import "fmt"

// TransactionType helps manage adding transactions (aka events) to the world transaction queue. It also assists
// in the using of transactions inside of System functions.
type TransactionType[T any] struct {
	world *World
	name  string
}

func NewTransactionType[T any](world *World, name string) *TransactionType[T] {
	if !world.AddTxName(name) {
		panic(fmt.Sprintf("Multiple definitions of transaction %q", name))
	}
	tx := &TransactionType[T]{
		world: world,
		name:  name,
	}
	return tx
}

func (t *TransactionType[T]) Name() string {
	return t.name
}

// AddToQueue adds a transaction with the given data to the world object. The transaction will be executed
// at the next game tick.
func (t *TransactionType[T]) AddToQueue(data *T) {
	t.world.AddTransaction(t.Name(), data)
}

// In extracts all the transactions from the transaction queue that match this TransactionType's name.
func (t *TransactionType[T]) In(tq *TransactionQueue) []*T {
	var txs []*T
	for _, tx := range tq.queue[t.Name()] {
		if val, ok := tx.(*T); ok {
			txs = append(txs, val)
		}
	}
	return txs
}
