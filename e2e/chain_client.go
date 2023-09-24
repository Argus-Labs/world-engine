package e2e

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/v3/assert"
	shardtypes "pkg.world.dev/world-engine/chain/x/shard/types"
	"testing"
)

type Chain struct {
	shardtypes.QueryClient
}

func NewChainClient(t *testing.T) Chain {
	cc, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return Chain{QueryClient: shardtypes.NewQueryClient(cc)}
}
