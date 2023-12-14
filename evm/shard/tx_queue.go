package shard

import (
	"cmp"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	"slices"
	"sync"
)

// TxQueue acts as a transaction queue. Transactions come in to the TxQueue with an epoch.
type TxQueue struct {
	lock       sync.Mutex
	ntx        map[string]*txQueue
	moduleAddr string
}

func NewTxQueue(moduleAddr string) *TxQueue {
	return &TxQueue{
		lock:       sync.Mutex{},
		ntx:        make(map[string]*txQueue),
		moduleAddr: moduleAddr,
	}
}

type txQueue struct {
	// txs are the transaction requests, indexed by epoch.
	txs map[uint64]*types.SubmitShardTxRequest
}

func (tc *TxQueue) GetRequestForNamespaceEpoch(ns string, epoch uint64) *types.SubmitShardTxRequest {
	return tc.ntx[ns].txs[epoch]
}

// AddTx first checks if there are already transactions stored for the epoch in the request.
// If there are, we simply append this request to txs.
// If there aren't yet, we append the epoch number to epochQueue, then append to the txs map.
func (tc *TxQueue) AddTx(namespace string, epoch, txID uint64, payload []byte) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	// if we have a brand-new namespace submitting transactions, we set up a new queue for it.
	if tc.ntx[namespace] == nil {
		tc.ntx[namespace] = &txQueue{
			txs: make(map[uint64]*types.SubmitShardTxRequest),
		}
	}

	// if we don't have a request for this epoch yet, instantiate one.
	if tc.ntx[namespace].txs[epoch] == nil {
		tc.ntx[namespace].txs[epoch] = &types.SubmitShardTxRequest{
			Sender:    tc.moduleAddr,
			Namespace: namespace,
			Epoch:     epoch,
			Txs:       make([]*types.Transaction, 0),
		}
	}

	// append the transaction data for this epoch.
	tc.ntx[namespace].txs[epoch].Txs = append(tc.ntx[namespace].txs[epoch].Txs, &types.Transaction{
		TxId:                 txID,
		GameShardTransaction: payload,
	})
}

// GetTxs simply copies the transactions in the outbox, clears the outbox, then returns the copy.
func (tc *TxQueue) GetTxs() []*types.SubmitShardTxRequest {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	var reqs []*types.SubmitShardTxRequest
	namespaces := sortMapKeys(tc.ntx)
	for _, namespace := range namespaces {
		txq := tc.ntx[namespace]
		epochs := sortMapKeys(txq.txs)
		for _, epoch := range epochs {
			reqs = append(reqs, txq.txs[epoch])
		}
	}
	clear(tc.ntx)
	return reqs
}

func sortMapKeys[S map[K]V, K cmp.Ordered, V any](m S) []K {
	keys := make([]K, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
