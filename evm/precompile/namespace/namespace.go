package namespace

import (
	"context"
	ethprecompile "github.com/berachain/polaris/eth/core/precompile"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	"pkg.world.dev/world-engine/evm/precompile/contracts/bindings/cosmos/precompile/namespace"
	"pkg.world.dev/world-engine/evm/x/namespace/types"
)

const name = "world_engine_namespace_registrar"

type Contract struct {
	ethprecompile.BaseContract
	types.MsgServer
	types.QueryServiceServer
}

func NewPrecompileContract(ms types.MsgServer, qs types.QueryServiceServer) *Contract {
	return &Contract{
		BaseContract: ethprecompile.NewBaseContract(
			namespace.NamespaceMetaData.ABI,
			common.BytesToAddress(authtypes.NewModuleAddress(name)),
		),
		MsgServer:          ms,
		QueryServiceServer: qs,
	}
}

func (c *Contract) Register(
	ctx context.Context,
	namespace string,
	grpcAddress string,
) (bool, error) {
	_, err := c.UpdateNamespace(ctx, &types.UpdateNamespaceRequest{
		Authority: "",
		Namespace: &types.Namespace{
			ShardName:    namespace,
			ShardAddress: grpcAddress,
		},
	})
	// if err == nil, this returns true, which means it succeeded.
	return err == nil, err
}

func (c *Contract) AddressForNamespace(ctx context.Context, namespace string) (string, error) {
	res, err := c.Address(ctx, &types.AddressRequest{Namespace: namespace})
	if err != nil {
		return "", err
	}
	return res.Address, nil
}
