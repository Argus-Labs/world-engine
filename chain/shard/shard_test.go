package shard

import (
	"context"
	"testing"

	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"gotest.tools/v3/assert"
)

func TestShard_Flush(t *testing.T) {
	sh := NewShardServer()
	msgs := [][]byte{[]byte("hello world"), []byte("goodbye world")}
	for _, msg := range msgs {
		res, err := sh.SubmitShardBatch(context.Background(), &shardv1.SubmitShardBatchRequest{Batch: msg})
		assert.NilError(t, err)
		assert.Check(t, res != nil)
	}

	flushed := sh.FlushMessages()
	for i, msg := range flushed {
		assert.DeepEqual(t, msg.Batch, msgs[i])
	}
	// msg queue should be empty after flushing.
	assert.Equal(t, len(sh.msgQueue), 0)
}
