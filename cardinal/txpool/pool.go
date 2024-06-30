package txpool

import (
	"context"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	ddotel "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/opentelemetry"
	ddtracer "gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

type TxMap map[types.MessageID][]TxData

type TxData struct {
	MsgID  types.MessageID
	Msg    any
	TxHash types.TxHash
	Tx     *sign.Transaction
	// EVMSourceTxHash is the tx hash of the EVM tx that triggered this tx.
	EVMSourceTxHash string
}

type TxPool struct {
	m         TxMap
	txsInPool int
	mux       *sync.Mutex
	tracer    trace.Tracer
}

func New() *TxPool {
	return &TxPool{
		m:      TxMap{},
		mux:    &sync.Mutex{},
		tracer: otel.Tracer("txpool"),
	}
}

func (t *TxPool) GetAmountOfTxs() int {
	return t.txsInPool
}

// GetEVMTxs gets all the txs in the queue that originated from the EVM.
// NOTE: this is called ONLY in the copied tx queue in world.doTick, so we do not need to use the mutex here.
func (t *TxPool) GetEVMTxs() []TxData {
	transactions := make([]TxData, 0)
	for _, txs := range t.m {
		// skip if theres nothing
		if len(txs) == 0 {
			continue
		}
		for _, tx := range txs {
			if tx.EVMSourceTxHash != "" {
				transactions = append(transactions, tx)
			}
		}
	}
	return transactions
}

func (t *TxPool) AddTransaction(id types.MessageID, v any, sig *sign.Transaction) types.TxHash {
	return t.addTransaction(id, v, sig, "")
}

func (t *TxPool) AddEVMTransaction(id types.MessageID, v any, sig *sign.Transaction, evmTxHash string) types.TxHash {
	return t.addTransaction(id, v, sig, evmTxHash)
}

func (t *TxPool) addTransaction(id types.MessageID, v any, sig *sign.Transaction, evmTxHash string) types.TxHash {
	t.mux.Lock()
	defer t.mux.Unlock()
	txHash := types.TxHash(sig.HashHex())
	t.m[id] = append(t.m[id], TxData{
		MsgID:           id,
		TxHash:          txHash,
		Msg:             v,
		Tx:              sig,
		EVMSourceTxHash: evmTxHash,
	})
	t.txsInPool++
	return txHash
}

func (t *TxPool) Transactions() TxMap {
	return t.m
}

// CopyTransactions returns a copy of the TxPool, and resets the state to 0 values.
func (t *TxPool) CopyTransactions(ctx context.Context) *TxPool {
	_, span := t.tracer.Start(ddotel.ContextWithStartOptions(ctx, ddtracer.Measured()), "txpool.copy-transactions")
	defer span.End()

	t.mux.Lock()
	defer t.mux.Unlock()

	cpy := *t
	t.reset()

	return &cpy
}

func (t *TxPool) reset() {
	t.m = TxMap{}
	t.txsInPool = 0
}

func (t *TxPool) ForID(id types.MessageID) []TxData {
	return t.m[id]
}
