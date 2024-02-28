package chain

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/v3/assert"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
)

type Chain struct {
	shard     shardtypes.QueryClient
	bank      banktypes.QueryClient
	namespace namespacetypes.QueryServiceClient
}

func newChainClient(t *testing.T) Chain {
	cc, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return Chain{
		shard:     shardtypes.NewQueryClient(cc),
		bank:      banktypes.NewQueryClient(cc),
		namespace: namespacetypes.NewQueryServiceClient(cc),
	}
}
