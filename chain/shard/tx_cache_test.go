package shard

import (
	"github.com/argus-labs/world-engine/chain/x/shard/types"
	"gotest.tools/v3/assert"
	"sync"
	"testing"
)

func TestAddTx(t *testing.T) {
	txq := TxQueue{
		lock:       sync.Mutex{},
		ntx:        make(NamespacedTxs, 0),
		outbox:     make([]*types.SubmitCardinalTxRequest, 0),
		moduleAddr: "foo",
	}

	namespace := "foobar"
	tick := uint64(3)
	txq.AddTx(namespace, tick, 2, []byte("hi"))
	txq.AddTx(namespace, tick, 2, []byte("hello"))
	// add a random bogus transaction for good measure
	txq.AddTx("bogus", 40, 2, []byte("HI"))
	// at this point, outbox should be empty, and there should be 2 txs in the queue.
	assert.Equal(t, len(txq.outbox), 0)
	txs := txq.TxsForNamespaceInTick(namespace, tick)
	assert.Equal(t, len(txs.Txs.Txs), 2)

	// now we add a tx from a different tick
	newTick := uint64(4)
	txq.AddTx(namespace, newTick, 3, []byte("foo"))
	assert.Equal(t, len(txq.outbox), 1)
	// txs in this namespace should only have one item
	assert.Equal(t, len(txq.ntx[namespace].txs), 1)
	// top of the queue should be new tick
	assert.Equal(t, txq.ntx[namespace].tickQueue.Dequeue(), newTick)
}
