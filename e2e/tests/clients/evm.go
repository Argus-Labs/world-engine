package clients

import (
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/v3/assert"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
)

type EVM struct {
	Shard     shardtypes.QueryClient
	Bank      banktypes.QueryClient
	Namespace namespacetypes.QueryServiceClient
}

func NewEVMClient(t *testing.T) *EVM {
	cc, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NilError(t, err)
	return &EVM{
		Shard:     shardtypes.NewQueryClient(cc),
		Bank:      banktypes.NewQueryClient(cc),
		Namespace: namespacetypes.NewQueryServiceClient(cc),
	}
}
