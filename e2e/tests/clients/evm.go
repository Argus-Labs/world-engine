package clients

import (
	"testing"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gotest.tools/v3/assert"

	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
	"pkg.world.dev/world-engine/rift/credentials"
	shard "pkg.world.dev/world-engine/rift/shard/v2"
)

type EVM struct {
	Shard     shardtypes.QueryClient
	Bank      banktypes.QueryClient
	Namespace namespacetypes.QueryServiceClient
}

type RiftClient struct {
	Rift shard.TransactionHandlerClient
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

func NewRiftClient(t *testing.T) *RiftClient {
	routerKey := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ01"
	cc, err := grpc.Dial(
		"localhost:9601",
		grpc.WithPerRPCCredentials(credentials.NewTokenCredential(routerKey)),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	assert.NilError(t, err)
	return &RiftClient{Rift: shard.NewTransactionHandlerClient(cc)}
}
