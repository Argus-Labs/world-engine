package cardinal

import (
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/sign"
)

// AnyTransaction is implemented by the return value of NewTransactionType and is used in RegisterTransactions; any
// transaction created by NewTransactionType can be registered with a World object via RegisterTransactions.
type AnyTransaction interface {
	Convert() transaction.ITransaction
}

// TransactionQueue contains the entire set of transactions that should be processed in a game tick. It is a parameter
// to a System function. Access the transactions of a particular type by using TransactionType.In
type TransactionQueue struct {
	impl *transaction.TxQueue
}

// TxData represents a single transaction.
type TxData[T any] struct {
	impl ecs.TxData[T]
}

// TransactionType represents a type of transaction that can be executed on the World object. The Msg struct represents
// the input for a specific transaction, and the Result struct represents the result of processing the transaction.
type TransactionType[Msg, Result any] struct {
	impl *ecs.TransactionType[Msg, Result]
}

// NewTransactionType creates a new instance of a TransactionType.
func NewTransactionType[Msg, Result any](name string) *TransactionType[Msg, Result] {
	return &TransactionType[Msg, Result]{
		impl: ecs.NewTransactionType[Msg, Result](name),
	}
}

// NewTransactionTypeWithEVMSupport creates a new instance of a TransactionType, with EVM transactions enabled.
// This allows this transaction to be sent from EVM smart contracts on the EVM base shard.
func NewTransactionTypeWithEVMSupport[Msg, Result any](name string) *TransactionType[Msg, Result] {
	return &TransactionType[Msg, Result]{
		impl: ecs.NewTransactionType[Msg, Result](name, ecs.WithTxEVMSupport[Msg, Result]),
	}
}

// AddToQueue is not meant to be used in production whatsoever, it is exposed here for usage in tests.
func (t *TransactionType[Msg, Result]) AddToQueue(world *World, data Msg, sigs ...*sign.SignedPayload) TxHash {
	txHash := t.impl.AddToQueue(world.implWorld, data, sigs...)
	return txHash
}

// AddError adds the given error to the transaction identified by the given hash. Multiple errors can be
// added to the same transaction hash.
func (t *TransactionType[Msg, Result]) AddError(wCtx WorldContext, hash TxHash, err error) {
	t.AddError(wCtx, hash, err)
}

// SetResult sets the result of the transaction identified by the given hash. Only one result may be associated
// with a transaction hash, so calling this multiple times will clobber previously set results.
func (t *TransactionType[Msg, Result]) SetResult(wCtx WorldContext, hash TxHash, result Result) {
	t.SetResult(wCtx, hash, result)
}

// GetReceipt returns the result (if any) and errors (if any) associated with the given hash. If false is returned,
// the hash is not recognized, so the returned result and errors will be empty.
func (t *TransactionType[Msg, Result]) GetReceipt(wCtx WorldContext, hash TxHash) (r Result, errs []error, ok bool) {
	return t.impl.GetReceipt(wCtx.getECSWorldContext(), hash)
}

func (t *TransactionType[Msg, Result]) ForEach(wCtx WorldContext, fn func(TxData[Msg]) (Result, error)) {
	adapterFn := func(ecsTxData ecs.TxData[Msg]) (Result, error) {
		adaptedTx := TxData[Msg]{impl: ecsTxData}
		return fn(adaptedTx)
	}
	t.impl.ForEach(wCtx.getECSWorldContext(), adapterFn)
}

// In returns the transactions in the given transaction queue that match this transaction's type.
func (t *TransactionType[Msg, Result]) In(wCtx WorldContext) []TxData[Msg] {
	ecsTxData := t.impl.In(wCtx.getECSWorldContext())
	out := make([]TxData[Msg], 0, len(ecsTxData))
	for _, tx := range ecsTxData {
		out = append(out, TxData[Msg]{
			impl: tx,
		})
	}
	return out
}

// Convert implements the AnyTransactionType interface which allows a TransactionType to be registered
// with a World via RegisterTransactions.
func (t *TransactionType[Msg, Result]) Convert() transaction.ITransaction {
	return t.impl
}

// Hash returns the hash of a specific transaction, which is used to associated results and errors with a specific
// transaction.
func (t *TxData[T]) Hash() TxHash {
	return t.impl.TxHash
}

// Value returns the input value of a transaction.
func (t *TxData[T]) Value() T {
	return t.impl.Value
}

// Sig returns the signature that was used to sign this transaction.
func (t *TxData[T]) Sig() *sign.SignedPayload {
	return t.impl.Sig
}
