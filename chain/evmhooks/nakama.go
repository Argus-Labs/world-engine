package evmhooks

import (
	"buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/argus-labs/argus/x/evm/types"
	"github.com/argus-labs/world-engine/hooks/evm"
)

var _ types.EvmHooks = NakamaHooks{}

func NewNakamaHooks(client sidecarv1grpc.NakamaClient, hooks ...evm.NakamaHook) types.EvmHooks {

	hookMap := make(map[common.Hash]evm.NakamaHookAction, len(hooks))
	for _, h := range hooks {
		hookMap[h.EventSignature] = h.Action
	}

	return NakamaHooks{
		nakamaClient: client,
		hooks:        hookMap,
	}
}

type NakamaHooks struct {
	nakamaClient sidecarv1grpc.NakamaClient
	hooks        map[common.Hash]evm.NakamaHookAction
}

func (n NakamaHooks) PostTxProcessing(ctx sdk.Context, msg core.Message, receipt *ethtypes.Receipt) error {
	if receipt != nil {
		for _, log := range receipt.Logs {
			for _, topic := range log.Topics {
				action, ok := n.hooks[topic]
				if ok {
					err := action(n.nakamaClient, ctx, msg, receipt)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}
