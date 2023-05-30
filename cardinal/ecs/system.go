package ecs

// TransactionQueue is a list of transactions that were queued sine the start of the
// last game tick.
type TransactionQueue struct {
	queue map[string][]interface{}
}

type System func(*TransactionQueue)
