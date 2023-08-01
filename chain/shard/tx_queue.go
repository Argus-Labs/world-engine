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
	outbox     []*types.SubmitCardinalTxRequest
	moduleAddr string
}

type txQueue struct {
	// tickQueue are the ticks waiting to be submitted to the blockchain
	tickQueue *queue.Queue[uint64]
	// txs are the transaction requests, indexed by tick.
	txs map[uint64]*types.SubmitCardinalTxRequest
}

// NamespacedTxs maps namespaces to a transaction queue.
type NamespacedTxs map[string]*txQueue

func (tc *TxQueue) TxsForNamespaceInTick(ns string, tick uint64) *types.SubmitCardinalTxRequest {
	return tc.ntx[ns].txs[tick]
}

// AddTx first checks if there are already transactions stored for the tick in the request.
// If there are, we simply append this request to txs.
// If there aren't yet, we append the tick number to tickQueue, then append to the txs map.
func (tc *TxQueue) AddTx(namespace string, tick, txID uint64, payload []byte) {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	// handle the case where this is a brand-new namespace
	if tc.ntx[namespace] == nil {
		tc.ntx[namespace] = &txQueue{
			tickQueue: queue.New[uint64](),
			txs:       make(map[uint64]*types.SubmitCardinalTxRequest),
		}
		tc.ntx[namespace].tickQueue.Enqueue(tick)
		tc.ntx[namespace].txs[tick] = &types.SubmitCardinalTxRequest{
			Sender:    tc.moduleAddr,
			Namespace: namespace,
			Tick:      tick,
			Txs:       []*types.Transaction{{TxId: txID, SignedPayload: payload}},
		}
		return
	}

	// namespace is present.
	// if there are no ticks in the queue, just enqueue it.
	if tc.ntx[namespace].tickQueue.Empty() {
		tc.ntx[namespace].tickQueue.Enqueue(tick)
		// check to see if this is a new tick. if it is, we enqueue it and
		// then send the previous ticks data to the outbox.
	} else if lastTick := tc.ntx[namespace].tickQueue.Peek(); lastTick != tick {
		// enqueue the new tick
		tc.ntx[namespace].tickQueue.Enqueue(tick)

		// since we know this is a new tick, we can dequeue and put the transaction request in the outbox.
		prev := tc.ntx[namespace].tickQueue.Dequeue()
		tc.outbox = append(tc.outbox, tc.ntx[namespace].txs[prev])
		delete(tc.ntx[namespace].txs, prev)
	}
	if tc.ntx[namespace].txs[tick] == nil {
		tc.ntx[namespace].txs[tick] = &types.SubmitCardinalTxRequest{
			Sender:    tc.moduleAddr,
			Namespace: namespace,
			Tick:      tick,
			Txs:       make([]*types.Transaction, 0),
		}
	}
	// finally, we append the transaction data to the transaction queue.
	tc.ntx[namespace].txs[tick].Txs = append(tc.ntx[namespace].txs[tick].Txs, &types.Transaction{
		TxId:          txID,
		SignedPayload: payload,
	})
}

// GetTxs simply copies the transactions in the outbox, clears the outbox, then returns the copy.
func (tc *TxQueue) GetTxs() []*types.SubmitCardinalTxRequest {
	tc.lock.Lock()
	defer tc.lock.Unlock()
	// copy the outbox.
	outboxCopy := make([]*types.SubmitCardinalTxRequest, len(tc.outbox))
	copy(outboxCopy, tc.outbox)
	// clear outbox, retain capacity.
	tc.outbox = tc.outbox[:0]
	return outboxCopy
}
