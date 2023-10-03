package public

type IReceipt interface {
	GetTxHash() TxHash
	GetErrors() []error
	GetResult() any
}

type IHistory interface {
	Size() uint64
	SetTick(tick uint64)
	SetResult(hash TxHash, result any)
	GetReceipt(hash TxHash) (rec IReceipt, ok bool)
	AddError(hash TxHash, err error)
	GetReceiptsForTick(tick uint64) ([]IReceipt, error)
	NextTick()
}
