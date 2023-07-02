package shard

import (
	"context"
	"testing"

	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

// TestShard_Flush tests that after submitting transactions to the shard server, they can be flushed out, and the
// queue is cleared.
func TestShard_Flush(t *testing.T) {
	sh := NewShardServer()
	batches := []*types.TransactionBatch{
		{Namespace: "foo", Tick: 4, Batch: []byte("hi")},
		{Namespace: "bar", Tick: 2, Batch: []byte("hello")},
	}
	for _, b := range batches {
		res, err := sh.SubmitShardBatch(
			context.Background(),
			&shardv1.SubmitShardBatchRequest{
				Namespace: b.Namespace,
				TickId:    b.Tick,
				Batch:     b.Batch,
			},
		)
		assert.NilError(t, err)
		assert.Check(t, res != nil)
	}

	flushed := sh.FlushMessages()
	for i, msg := range flushed {
		assert.DeepEqual(t, msg.TransactionBatch, batches[i])
	}
	// msg queue should be empty after flushing.
	assert.Equal(t, len(sh.msgQueue), 0)
}
