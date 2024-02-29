package sequencer

import (
	"pkg.world.dev/world-engine/assert"
	"testing"
)

// TestAddTx tests that txs can be added to the queue, and then flushed sorted by namespace & epoch.
func TestAddTx(t *testing.T) {
	txq := NewTxQueue("0xfoo")

	namespace := "foobar"
	epoch := uint64(3)
	epoch2 := uint64(5)
	txq.AddTx(namespace, epoch, 10, 15, []byte("hi"))
	txq.AddTx(namespace, epoch, 10, 3, []byte("hello"))
	txq.AddTx(namespace, epoch2, 20, 2, []byte("bye"))
	txq.AddTx("bogus", 40, 20, 2, []byte("HI"))
	txs := txq.FlushTxQueue()
	assert.Len(t, txs, 3) // should be 3 txs, as its partitioned by namespace and then by epoch

	assert.Equal(t, txs[0].Namespace, "bogus") // should be sorted
	assert.Equal(t, txs[1].Namespace, namespace)
	// epochs should be sorted
	assert.Equal(t, txs[1].Epoch, epoch)
	assert.Equal(t, txs[2].Epoch, epoch2)
}

func TestAddInitMsg(t *testing.T) {
	txq := NewTxQueue("0xfoo")
	namespace := "foo"
	addr := "hi:123"

	namespace2 := "hi"
	addr2 := "foo:123"

	txq.AddInitMsg(namespace, addr)
	txq.AddInitMsg(namespace2, addr2)

	inits := txq.FlushInitQueue()
	assert.Len(t, inits, 2)
	assert.Equal(t, inits[0].Namespace.ShardName, namespace)
	assert.Equal(t, inits[0].Namespace.ShardAddress, addr)

	assert.Equal(t, inits[1].Namespace.ShardName, namespace2)
	assert.Equal(t, inits[1].Namespace.ShardAddress, addr2)

	txq.AddInitMsg("foo", "bar")
	assert.Len(t, txq.FlushInitQueue(), 1)
}
