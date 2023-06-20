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

	maxArgs = 2
)

type Contract struct {
	ethprecompile.BaseContract
	r router.Router
}

// NewPrecompileContract
// TODO(technicallyty): decide address
func NewPrecompileContract(r router.Router) ethprecompile.StatefulImpl {
	return &Contract{
		BaseContract: ethprecompile.NewBaseContract(
			generated.RouterMetaData.ABI,
			cosmlib.AccAddressToEthAddress(authtypes.NewModuleAddress(name)),
		),
		r: r,
	}
}

func (c *Contract) PrecompileMethods() ethprecompile.Methods {
	return ethprecompile.Methods{
		{
			AbiSig:  "Send(bytes,string)",
			Execute: c.Send,
		},
	}
}

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
	namespace, ok := utils.GetAs[string](args[1])
	if !ok {
		return nil, precompile.ErrInvalidString
	}

	result, err := c.r.Send(ctx, namespace, caller.String(), payload)
	if err != nil {
		return nil, err
	}
	res := generated.IRouterResponse{
		Code:    big.NewInt(int64(result.Code)),
		Message: result.Message,
	}
	return []any{res}, nil
}

func matchArgs(max, actual int) error {
	if max != actual {
		return fmt.Errorf("wanted %d args, got %d", max, actual)
	}
	return nil
}
