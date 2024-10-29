package sequencer

import (
	"cmp"
	"slices"
	"sync"

	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	"pkg.world.dev/world-engine/evm/x/shard/types"
)

// TxQueue acts as a transaction queue. Transactions come in to the TxQueue with an epoch.
type TxQueue struct {
	lock       sync.Mutex
	txQueue    map[string]map[uint64]*types.SubmitShardTxRequest
	initQueue  []*namespacetypes.UpdateNamespaceRequest
	moduleAddr string
}

func NewTxQueue(moduleAddr string) *TxQueue {
	return &TxQueue{
		lock:       sync.Mutex{},
		txQueue:    make(map[string]map[uint64]*types.SubmitShardTxRequest),
		initQueue:  make([]*namespacetypes.UpdateNamespaceRequest, 0),
		moduleAddr: moduleAddr,
	}
}

func (tc *TxQueue) AddInitMsg(namespace, routerAddr string) error {
	tc.lock.Lock()
	defer tc.lock.Unlock()

	// Validate the request
	req := &namespacetypes.UpdateNamespaceRequest{
		Authority: tc.moduleAddr,
		Namespace: &namespacetypes.Namespace{
			ShardName:    namespace,
			ShardAddress: routerAddr,
		},
	}
	if err := req.ValidateBasic(); err != nil {
		return err
	}

	tc.initQueue = append(tc.initQueue, &namespacetypes.UpdateNamespaceRequest{
		Authority: tc.moduleAddr,
		Namespace: &namespacetypes.Namespace{
			ShardName:    namespace,
			ShardAddress: routerAddr,
		},
	})

	return nil
}

// AddTx adds a transaction to the queue.
func (tc *TxQueue) AddTx(namespace string, epoch uint64, unixTimestamp uint64, txID string, payload []byte) error {
	tc.lock.Lock()
	defer tc.lock.Unlock()

	if tc.txQueue[namespace] == nil {
		tc.txQueue[namespace] = make(map[uint64]*types.SubmitShardTxRequest)
	}

	// if we don't have a request for this epoch yet, instantiate one.
	if tc.txQueue[namespace][epoch] == nil {
		// Validate the request
		req := &types.SubmitShardTxRequest{
			Sender:        tc.moduleAddr,
			Namespace:     namespace,
			Epoch:         epoch,
			UnixTimestamp: unixTimestamp,
			Txs:           make([]*types.Transaction, 0),
		}
		if err := req.ValidateBasic(); err != nil {
			return err
		}

		tc.txQueue[namespace][epoch] = req
	}

	// append the transaction data for this epoch.
	tc.txQueue[namespace][epoch].Txs = append(tc.txQueue[namespace][epoch].Txs, &types.Transaction{
		TxId:                 txID,
		GameShardTransaction: payload,
	})

	return nil
}

// FlushTxQueue gets all currently queued transactions sorted by namespace and by transaction ID, and then clears the
// queue.
func (tc *TxQueue) FlushTxQueue() []*types.SubmitShardTxRequest {
	tc.lock.Lock()
	defer tc.lock.Unlock()

	var reqs []*types.SubmitShardTxRequest
	namespaces := sortMapKeys(tc.txQueue)
	for _, namespace := range namespaces {
		txq := tc.txQueue[namespace]
		epochs := sortMapKeys(txq)
		for _, epoch := range epochs {
			reqs = append(reqs, txq[epoch])
		}
	}

	clear(tc.txQueue)

	return reqs
}

func (tc *TxQueue) FlushInitQueue() []*namespacetypes.UpdateNamespaceRequest {
	tc.lock.Lock()
	defer tc.lock.Unlock()

	if len(tc.initQueue) == 0 {
		return nil
	}

	reqs := tc.initQueue
	tc.initQueue = make([]*namespacetypes.UpdateNamespaceRequest, 0)

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
