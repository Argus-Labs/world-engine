package sequencer

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
)

// TestAddTx tests that txs can be added to the queue, and then flushed sorted by namespace & epoch.
func TestAddTx(t *testing.T) {
	txq := NewTxQueue("cosmos1n6j7gnld9yxfyh6tflxhjjmt404zruuaf73t08")

	namespace := "foobar"
	epoch := uint64(3)
	epoch2 := uint64(5)
	assert.NilError(t, txq.AddTx(namespace, epoch, 10, "15", []byte("hi")))
	assert.NilError(t, txq.AddTx(namespace, epoch, 10, "3", []byte("hello")))
	assert.NilError(t, txq.AddTx(namespace, epoch2, 20, "2", []byte("bye")))
	assert.NilError(t, txq.AddTx("bogus", 40, 20, "2", []byte("HI")))
	txs := txq.FlushTxQueue()
	assert.Len(t, txs, 3) // should be 3 txs, as its partitioned by namespace and then by epoch

	assert.Equal(t, txs[0].Namespace, "bogus") // should be sorted
	assert.Equal(t, txs[1].Namespace, namespace)
	// epochs should be sorted
	assert.Equal(t, txs[1].Epoch, epoch)
	assert.Equal(t, txs[2].Epoch, epoch2)
}

func TestAddInitMsg(t *testing.T) {
	txq := NewTxQueue("cosmos1n6j7gnld9yxfyh6tflxhjjmt404zruuaf73t08")
	namespace := "foo"
	addr := "hi:4040"

	namespace2 := "hi"
	addr2 := "foo:4040"

	assert.NilError(t, txq.AddInitMsg(namespace, addr))
	assert.NilError(t, txq.AddInitMsg(namespace2, addr2))

	inits := txq.FlushInitQueue()
	assert.Len(t, inits, 2)
	assert.Equal(t, inits[0].Namespace.ShardName, namespace)
	assert.Equal(t, inits[0].Namespace.ShardAddress, addr)

	assert.Equal(t, inits[1].Namespace.ShardName, namespace2)
	assert.Equal(t, inits[1].Namespace.ShardAddress, addr2)

	assert.NilError(t, txq.AddInitMsg("foo", "bar:4040"))
	assert.Len(t, txq.FlushInitQueue(), 1)
}
