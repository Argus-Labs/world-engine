package shard

import (
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/chain/x/shard/types"
	"sync"
	"testing"
)

func TestAddTx(t *testing.T) {
	txq := TxQueue{
		lock:       sync.Mutex{},
		ntx:        make(NamespacedTxs, 0),
		outbox:     make([]*types.SubmitShardTxRequest, 0),
		moduleAddr: "foo",
	}

	namespace := "foobar"
	epoch := uint64(3)
	txq.AddTx(namespace, epoch, 2, []byte("hi"))
	txq.AddTx(namespace, epoch, 2, []byte("hello"))
	// add a random bogus transaction for good measure
	txq.AddTx("bogus", 40, 2, []byte("HI"))
	// at this point, outbox should be empty, and there should be 2 txs in the queue.
	assert.Equal(t, len(txq.outbox), 0)
	req := txq.GetRequestForNamespaceEpoch(namespace, epoch)
	assert.Equal(t, len(req.Txs), 2)

	// now we add a tx from a different epoch
	newEpoch := uint64(4)
	txq.AddTx(namespace, newEpoch, 3, []byte("foo"))
	assert.Equal(t, len(txq.outbox), 1)
	// txs in this namespace should only have one item
	assert.Equal(t, len(txq.ntx[namespace].txs), 1)
	// top of the queue should be new epoch
	assert.Equal(t, txq.ntx[namespace].epochQueue.Dequeue(), newEpoch)
}
