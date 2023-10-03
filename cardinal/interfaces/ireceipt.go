package interfaces

type IReceipt interface {
	GetTxHash() TxHash
	GetErrors() []error
	GetResult() any
}
