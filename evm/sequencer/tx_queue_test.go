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
	txs := txq.GetTxs()
	assert.Len(t, txs, 3) // should be 3 txs, as its partitioned by namespace and then by epoch

	assert.Equal(t, txs[0].Namespace, "bogus") // should be sorted
	assert.Equal(t, txs[1].Namespace, namespace)
	// epochs should be sorted
	assert.Equal(t, txs[1].Epoch, epoch)
	assert.Equal(t, txs[2].Epoch, epoch2)
}
