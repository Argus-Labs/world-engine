package e2ebenchmark

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/v3/assert"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	shardtypes "pkg.world.dev/world-engine/chain/x/shard/types"
)

type Chain struct {
	shard shardtypes.QueryClient
	bank  banktypes.QueryClient
}

func newChainClient(t *testing.T) Chain {
	cc, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return Chain{shard: shardtypes.NewQueryClient(cc), bank: banktypes.NewQueryClient(cc)}
}
