package router

import (
	"context"
	"fmt"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog/log"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"
	"pkg.berachain.dev/polaris/eth/core/vm"
	generated "pkg.world.dev/world-engine/evm/precompile/contracts/bindings/cosmos/precompile/router"
	"pkg.world.dev/world-engine/evm/router"
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
	messageID string,
	namespace string,
) (bool, error) {
	log.Logger.Debug().Msg("inside SendMessage precompile function called")
	pCtx := vm.UnwrapPolarContext(ctx)
	if c.rtr != nil {
		err := c.rtr.SendMessage(ctx, namespace, pCtx.MsgSender().String(), messageID, message)
		if err != nil {
			log.Logger.Err(err).Msg("failed to queue message in router")
			return false, err
		}
		log.Logger.Debug().Msgf("successfully queued message to %s from %s", namespace, pCtx.MsgSender().String())
		return true, nil
	}
	log.Logger.Debug().Msg("the precompile had a nil Router")
	return false, fmt.Errorf("nil router")
}

func (c *Contract) MessageResult(ctx context.Context, evmTxHash string) ([]byte, string, uint32, error) {
	resultBz, resultErr, resultCode, err := c.rtr.MessageResult(ctx, evmTxHash)
	if err != nil {
		log.Error().Err(err).Msgf("failed to get msg result")
		return nil, "", 0, err
	}
	log.Debug().Msgf("successfully retrieved result: code %d, errors %q", resultCode, resultErr)
	return resultBz, resultErr, resultCode, nil
}

func (c *Contract) Query(
	ctx context.Context,
	request []byte,
	resource, namespace string,
) ([]byte, error) {
	log.Debug().Msgf("got query request for %s", namespace)
	return c.rtr.Query(ctx, request, resource, namespace)
}
