package sequencer

import (
	"cmp"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	"slices"
	"sync"
)

// TxQueue acts as a transaction queue. Transactions come in to the TxQueue with an epoch.
type TxQueue struct {
	lock       sync.Mutex
	queues     map[string]map[uint64]*types.SubmitShardTxRequest
	moduleAddr string
}

func NewTxQueue(moduleAddr string) *TxQueue {
	return &TxQueue{
		lock:       sync.Mutex{},
		queues:     make(map[string]map[uint64]*types.SubmitShardTxRequest),
		moduleAddr: moduleAddr,
	}
}

type txQueue struct {
	// txs are the transaction requests, indexed by epoch.
	txs map[uint64]*types.SubmitShardTxRequest
}

// AddTx adds a transaction to the queue.
func (tc *TxQueue) AddTx(namespace string, epoch, txID uint64, payload []byte) {
	tc.lock.Lock()
	defer tc.lock.Unlock()

	if tc.queues[namespace] == nil {
		tc.queues[namespace] = make(map[uint64]*types.SubmitShardTxRequest)
	}

	// if we don't have a request for this epoch yet, instantiate one.
	if tc.queues[namespace][epoch] == nil {
		tc.queues[namespace][epoch] = &types.SubmitShardTxRequest{
			Sender:    tc.moduleAddr,
			Namespace: namespace,
			Epoch:     epoch,
			Txs:       make([]*types.Transaction, 0),
		}
	}

	// append the transaction data for this epoch.
	tc.queues[namespace][epoch].Txs = append(tc.queues[namespace][epoch].Txs, &types.Transaction{
		TxId:                 txID,
		GameShardTransaction: payload,
	})
}

// GetTxs gets all currently queued transactions sorted by namespace and by transaction ID, and then clears the queue.
func (tc *TxQueue) GetTxs() []*types.SubmitShardTxRequest {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	var reqs []*types.SubmitShardTxRequest
	namespaces := sortMapKeys(tc.queues)
	for _, namespace := range namespaces {
		txq := tc.queues[namespace]
		epochs := sortMapKeys(txq)
		for _, epoch := range epochs {
			reqs = append(reqs, txq[epoch])
		}
	}
	clear(tc.queues)
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
