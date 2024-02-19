package txpool

import (
	"pkg.world.dev/world-engine/cardinal/types"
	"sync"

	"pkg.world.dev/world-engine/sign"
)

type TxQueue struct {
	m          TxMap
	txsInQueue int
	mux        *sync.Mutex
}

func NewTxQueue() *TxQueue {
	return &TxQueue{
		m:   TxMap{},
		mux: &sync.Mutex{},
	}
}

func (t *TxQueue) GetAmountOfTxs() int {
	return t.txsInQueue
}

// GetEVMTxs gets all the txs in the queue that originated from the EVM.
// NOTE: this is called ONLY in the copied tx queue in world.Tick, so we do not need to use the mutex here.
func (t *TxQueue) GetEVMTxs() []TxData {
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

func (t *TxQueue) AddTransaction(id types.MessageID, v any, sig *sign.Transaction) types.TxHash {
	return t.addTransaction(id, v, sig, "")
}

func (t *TxQueue) AddEVMTransaction(id types.MessageID, v any, sig *sign.Transaction, evmTxHash string) types.TxHash {
	return t.addTransaction(id, v, sig, evmTxHash)
}

func (t *TxQueue) addTransaction(id types.MessageID, v any, sig *sign.Transaction, evmTxHash string) types.TxHash {
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
	t.txsInQueue++
	return txHash
}

func (t *TxQueue) Transactions() TxMap {
	return t.m
}

// CopyTransactions returns a copy of the TxQueue, and resets the state to 0 values.
func (t *TxQueue) CopyTransactions() *TxQueue {
	t.mux.Lock()
	defer t.mux.Unlock()
	cpy := *t
	t.reset()
	return &cpy
}

func (t *TxQueue) reset() {
	t.m = TxMap{}
	t.txsInQueue = 0
}

func (t *TxQueue) ForID(id types.MessageID) []TxData {
	return t.m[id]
}

type TxMap map[types.MessageID][]TxData

type TxData struct {
	MsgID  types.MessageID
	Msg    any
	TxHash types.TxHash
	Tx     *sign.Transaction
	// EVMSourceTxHash is the tx hash of the EVM tx that triggered this tx.
	EVMSourceTxHash string
}
