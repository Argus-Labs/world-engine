package shard

import (
	"github.com/argus-labs/world-engine/chain/x/shard/types"
	"github.com/zyedidia/generic/queue"
	"sync"
)

// TxQueue acts as a transaction queue. Transactions come in to the TxQueue with at tick.
type TxQueue struct {
	lock       sync.Mutex
	ntx        NamespacedTxs
	outbox     []*types.SubmitShardTxRequest
	moduleAddr string
}

type txQueue struct {
	// tickQueue are the ticks waiting to be submitted to the blockchain
	tickQueue *queue.Queue[uint64]
	// txs are the transaction requests, indexed by tick.
	txs map[uint64]*types.SubmitShardTxRequest
}

// NamespacedTxs maps namespaces to a transaction queue.
type NamespacedTxs map[string]*txQueue

func (tc *TxQueue) GetRequestForNamespaceTick(ns string, tick uint64) *types.SubmitShardTxRequest {
	return tc.ntx[ns].txs[tick]
}

// AddTx first checks if there are already transactions stored for the tick in the request.
// If there are, we simply append this request to txs.
// If there aren't yet, we append the tick number to tickQueue, then append to the txs map.
func (tc *TxQueue) AddTx(namespace string, tick, txID uint64, payload []byte) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	// if we have a brand-new namespace submitting transactions, we setup a new queue for it.
	if tc.ntx[namespace] == nil {
		tc.ntx[namespace] = &txQueue{
			tickQueue: queue.New[uint64](),
			txs:       make(map[uint64]*types.SubmitShardTxRequest),
		}
	}

	// if the queue is empty, enqueue the tick number.
	if tc.ntx[namespace].tickQueue.Empty() {
		tc.ntx[namespace].tickQueue.Enqueue(tick)
	} else if lastTick := tc.ntx[namespace].tickQueue.Peek(); lastTick != tick {
		// if the queue is not empty, and the submitting tick is not the same as the last seen one:
		// 1. enqueue the new tick
		// 2. dequeue the top tick, and submit its transactions to the outbox.
		// 3. delete the transactions from the map.
		tc.ntx[namespace].tickQueue.Enqueue(tick)
		prev := tc.ntx[namespace].tickQueue.Dequeue()
		tc.outbox = append(tc.outbox, tc.ntx[namespace].txs[prev])
		delete(tc.ntx[namespace].txs, prev)
	}

	// if we don't have a request for this tick yet, instantiate one.
	if tc.ntx[namespace].txs[tick] == nil {
		tc.ntx[namespace].txs[tick] = &types.SubmitShardTxRequest{
			Sender:    tc.moduleAddr,
			Namespace: namespace,
			Tick:      tick,
			Txs:       make([]*types.Transaction, 0),
		}
	}

	// append the transaction data for this tick.
	tc.ntx[namespace].txs[tick].Txs = append(tc.ntx[namespace].txs[tick].Txs, &types.Transaction{
		TxId:          txID,
		SignedPayload: payload,
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
