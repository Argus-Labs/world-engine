package transaction

import (
	"sync"

	"pkg.world.dev/world-engine/cardinal/interfaces"
	"pkg.world.dev/world-engine/sign"
)

type TxQueue struct {
	m   txMap
	mux *sync.Mutex
}

func NewTxQueue() *TxQueue {
	return &TxQueue{
		m:   txMap{},
		mux: &sync.Mutex{},
	}
}

func (t *TxQueue) GetAmountOfTxs() int {
	t.mux.Lock()
	defer t.mux.Unlock()
	acc := 0
	for _, v := range t.m {
		acc += len(v)
	}
	return acc
}

func (t *TxQueue) AddTransaction(id interfaces.TransactionTypeID, v any, sig *sign.SignedPayload) interfaces.TxHash {
	t.mux.Lock()
	defer t.mux.Unlock()
	txHash := interfaces.TxHash(sig.HashHex())
	t.m[id] = append(t.m[id], interfaces.TxAny{
		TxHash: txHash,
		Value:  v,
		Sig:    sig,
	})
	return txHash
}

func (t *TxQueue) CopyTransaction() interfaces.ITxQueue {
	t.mux.Lock()
	defer t.mux.Unlock()
	cpy := &TxQueue{
		m: t.m,
	}
	t.m = txMap{}
	return cpy
}

func (t *TxQueue) ForID(id interfaces.TransactionTypeID) []interfaces.TxAny {
	return t.m[id]
}

type txMap map[interfaces.TransactionTypeID][]interfaces.TxAny
