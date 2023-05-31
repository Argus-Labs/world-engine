package ecs

// TransactionQueue is a list of transactions that were queued since the start of the
// last game tick.
type TransactionQueue struct {
	queue map[string][]any
}

type System func(*TransactionQueue)
