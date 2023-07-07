package router

import (
	"context"
	"fmt"
	"math/big"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	cosmlib "pkg.berachain.dev/polaris/cosmos/lib"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/lib/utils"

	"github.com/argus-labs/world-engine/chain/precompile"
	"github.com/argus-labs/world-engine/chain/router"
	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile/router"
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

func (c *Contract) PrecompileMethods() ethprecompile.Methods {
	return ethprecompile.Methods{
		{
			AbiSig:  "Send(bytes,uint64,string)",
			Execute: c.Send,
		},
	}
}

// Send implements the Send precompile function in router.sol.
// function Send(bytes calldata message, uint64 messageID, string calldata namespace) external returns (Response memory);
func (c *Contract) Send(
	ctx context.Context,
	_ ethprecompile.EVM,
	caller common.Address,
	_ *big.Int,
	_ bool,
	args ...any,
) ([]any, error) {
	if err := matchArgs(maxArgs, len(args)); err != nil {
		return nil, err
	}
	payload, ok := utils.GetAs[[]byte](args[0])
	if !ok {
		return nil, precompile.ErrInvalidBytes
	}
	msgID, ok := utils.GetAs[uint64](args[1])
	if !ok {
		return nil, precompile.ErrInvalidUint64
	}
	namespace, ok := utils.GetAs[string](args[2])
	if !ok {
		return nil, precompile.ErrInvalidString
	}

	_, err := c.rtr.Send(ctx, namespace, caller.String(), msgID, payload)

	return []any{}, err
}

func matchArgs(max, actual int) error {
	if max != actual {
		return fmt.Errorf("wanted %d args, got %d", max, actual)
	}
	return nil
}
