package router

import (
	"context"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile/router"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/eth/core/vm"
	"pkg.world.dev/world-engine/chain/router"
)

const name = "world_engine_router"

type Contract struct {
	ethprecompile.BaseContract
	rtr router.Router
}

// NewPrecompileContract returns a new instance of the Router precompile.
func NewPrecompileContract(r router.Router) *Contract {
	return &Contract{
		BaseContract: ethprecompile.NewBaseContract(
			generated.RouterMetaData.ABI,
			common.BytesToAddress(authtypes.NewModuleAddress(name)),
		),
		rtr: r,
	}
}

// SendMessage implements the sendMessage precompile function in router.sol.
func (c *Contract) SendMessage(
	ctx context.Context,
	message []byte,
	messageID uint64,
	namespace string,
) ([]byte, error) {
	pCtx := vm.UnwrapPolarContext(ctx)
	_, err := c.rtr.SendMessage(ctx, namespace, pCtx.MsgSender().String(), messageID, message)
	return nil, err
}

func (c *Contract) Query(
	ctx context.Context,
	request []byte,
	resource, namespace string,
) ([]byte, error) {
	return c.rtr.Query(ctx, request, resource, namespace)
}
