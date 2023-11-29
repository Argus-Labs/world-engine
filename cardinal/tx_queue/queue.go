package tx_queue

import (
	"sync"

	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

type TxQueue struct {
	m          txMap
	txsInQueue int
	mux        *sync.Mutex
}

func NewTxQueue() *TxQueue {
	return &TxQueue{
		m:   txMap{},
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

func (t *TxQueue) AddTransaction(id message.TypeID, v any, sig *sign.Transaction) message.TxHash {
	return t.addTransaction(id, v, sig, "")
}

func (t *TxQueue) AddEVMTransaction(id message.TypeID, v any, sig *sign.Transaction, evmTxHash string) message.TxHash {
	return t.addTransaction(id, v, sig, evmTxHash)
}

func (t *TxQueue) addTransaction(id message.TypeID, v any, sig *sign.Transaction, evmTxHash string) message.TxHash {
	t.mux.Lock()
	defer t.mux.Unlock()
	txHash := message.TxHash(sig.HashHex())
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

// CopyTransactions returns a copy of the TxQueue, and resets the state to 0 values.
func (t *TxQueue) CopyTransactions() *TxQueue {
	t.mux.Lock()
	defer t.mux.Unlock()
	cpy := *t
	t.reset()
	return &cpy
}

func (t *TxQueue) reset() {
	t.m = txMap{}
	t.txsInQueue = 0
}

func (t *TxQueue) ForID(id message.TypeID) []TxData {
	return t.m[id]
}

type txMap map[message.TypeID][]TxData

type TxData struct {
	MsgID  message.TypeID
	Msg    any
	TxHash message.TxHash
	Tx     *sign.Transaction
	// EVMSourceTxHash is the tx hash of the EVM tx that triggered this tx.
	EVMSourceTxHash string
}
