package DarkForest

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gogo/protobuf/proto"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/lib/utils"

	"github.com/argus-labs/world-engine/chain/precompile"
	generated "pkg.berachain.dev/polaris/contracts/bindings/cosmos/precompile"
)

type Router interface {
	Send(string, proto.Message) (proto.Message, error)
}

type Contract struct {
	precompile.BaseContract
	r Router
}

func NewPrecompileContract(r Router) ethprecompile.StatefulImpl {
	return &Contract{
		BaseContract: precompile.NewBaseContract(generated.DarkForestMetaData.ABI,
			common.Address{}),
		r: nil,
	}
}

func (c *Contract) PrecompileMethods() ethprecompile.Methods {
	return ethprecompile.Methods{
		{
			AbiSig: "",
		},
	}
}

func (c *Contract) SendEnergy(
	ctx context.Context,
	_ ethprecompile.EVM,
	caller common.Address,
	value *big.Int,
	readonly bool,
	args ...any,
) ([]any, error) {
	msg, ok := utils.GetAs[generated.DarkForestMsgSendEnergy](args[0])
	if !ok {
		typ, _ := utils.GetAs[any](args[0])
		return nil, fmt.Errorf("SendEnergy argument was of type %T, needed type %T", typ, generated.DarkForestMsgSendEnergy{})
	}
	
}
