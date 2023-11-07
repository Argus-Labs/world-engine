package shard

import (
	"github.com/zyedidia/generic/queue"
	"pkg.world.dev/world-engine/chain/x/shard/types"
	"sync"
)

// TxQueue acts as a transaction queue. Transactions come in to the TxQueue with an epoch.
type TxQueue struct {
	lock       sync.Mutex
	ntx        NamespacedTxs
	outbox     []*types.SubmitShardTxRequest
	moduleAddr string
}

type txQueue struct {
	// epochQueue are the epochs waiting to be submitted to the blockchain
	epochQueue *queue.Queue[uint64]
	// txs are the transaction requests, indexed by epoch.
	txs map[uint64]*types.SubmitShardTxRequest
}

// NamespacedTxs maps namespaces to a transaction queue.
type NamespacedTxs map[string]*txQueue

func (tc *TxQueue) GetRequestForNamespaceEpoch(ns string, epoch uint64) *types.SubmitShardTxRequest {
	return tc.ntx[ns].txs[epoch]
}

// AddTx first checks if there are already transactions stored for the epoch in the request.
// If there are, we simply append this request to txs.
// If there aren't yet, we append the epoch number to epochQueue, then append to the txs map.
func (tc *TxQueue) AddTx(namespace string, epoch, txID uint64, payload []byte) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	// if we have a brand-new namespace submitting transactions, we setup a new queue for it.
	if tc.ntx[namespace] == nil {
		tc.ntx[namespace] = &txQueue{
			epochQueue: queue.New[uint64](),
			txs:        make(map[uint64]*types.SubmitShardTxRequest),
		}
	}

	// if the queue is empty, enqueue the epoch number.
	if tc.ntx[namespace].epochQueue.Empty() {
		tc.ntx[namespace].epochQueue.Enqueue(epoch)
	} else if lastEpoch := tc.ntx[namespace].epochQueue.Peek(); lastEpoch != epoch {
		// if the queue is not empty, and the submitting epoch is not the same as the last seen one:
		// 1. enqueue the new epoch
		// 2. dequeue the top epoch, and submit its transactions to the outbox.
		// 3. delete the transactions from the map.
		tc.ntx[namespace].epochQueue.Enqueue(epoch)
		prev := tc.ntx[namespace].epochQueue.Dequeue()
		tc.outbox = append(tc.outbox, tc.ntx[namespace].txs[prev])
		delete(tc.ntx[namespace].txs, prev)
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
	// copy the outbox.
	outboxCopy := make([]*types.SubmitShardTxRequest, len(tc.outbox))
	copy(outboxCopy, tc.outbox)
	// clear outbox, retain capacity.
	tc.outbox = tc.outbox[:0]
	return outboxCopy
}
