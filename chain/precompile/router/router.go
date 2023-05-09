package router

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/lib/utils"

	"github.com/argus-labs/world-engine/chain/precompile"
	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile"
)

const (
	CodeSuccess = iota
	CodeFailed
	CodeTimedOut
)

type Result struct {
	Code    uint64
	Message string
}

type Rtr interface {
	Send(namespace string, sender string, msg []byte) (Result, error)
}

type Contract struct {
	precompile.BaseContract
	r Rtr
}

// NewPrecompileContract
// TODO(technicallyty): decide address
func NewPrecompileContract(r Rtr) ethprecompile.StatefulImpl {
	return &Contract{
		BaseContract: precompile.NewBaseContract(generated.RouterMetaData.ABI, common.Address{}),
		r:            r,
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
	value *big.Int,
	readonly bool,
	args ...any,
) ([]any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("expected 2 args, got %d", len(args))
	}
	payload, ok := utils.GetAs[[]byte](args[0])
	if !ok {
		return nil, fmt.Errorf("expected bytes for arg[0]")
	}
	namespace, ok := utils.GetAs[string](args[1])
	if !ok {
		return nil, precompile.ErrInvalidString
	}

	result, err := c.r.Send(namespace, caller.String(), payload)
	if err != nil {
		return nil, err
	}
	res := generated.IRouterResponse{
		Code:    big.NewInt(int64(result.Code)),
		Message: result.Message,
	}
	return []any{res}, nil
}
