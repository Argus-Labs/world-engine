package shard

import (
	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"gotest.tools/v3/assert"
	"testing"
)

func TestShard(t *testing.T) {
	addr, err := sdk.AccAddressFromBech32("cosmos1jv94sqypjg9x0gwcl2mvy7ffwemnpwdqu0lxtk")
	assert.NilError(t, err)
	sh := NewShardServer(addr)
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

}
