package router

import (
	"context"
	"github.com/argus-labs/world-engine/chain/router"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile/router"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/eth/core/vm"
)

const (
	name = "world_engine_router"

	maxArgs = 3
)

type Contract struct {
	ethprecompile.BaseContract
	rtr router.Router
}

// NewPrecompileContract returns a new instance of the Router precompile.
func NewPrecompileContract(r router.Router) ethprecompile.StatefulImpl {
	return &Contract{
		BaseContract: ethprecompile.NewBaseContract(
			generated.RouterMetaData.ABI,
			cosmlib.AccAddressToEthAddress(authtypes.NewModuleAddress(name)),
		),
		rtr: r,
	}
}

// Send implements the Send precompile function in router.sol.
func (c *Contract) Send(
	ctx context.Context,
	msg []byte,
	msgID uint64,
	namespace string,
) error {
	pCtx := vm.UnwrapPolarContext(ctx)
	_, err := c.rtr.Send(ctx, namespace, pCtx.MsgSender().String(), msgID, msg)
	return err
}
