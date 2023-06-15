package evm

type TxHandler interface {
	UnmarshalAndSubmit(bz []byte, submitFn func(name string, v any)) error
}

type TransactionQueuer interface {
	AddTransaction(name string, v any)
}
