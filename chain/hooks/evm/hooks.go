package evm

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"

	g1 "buf.build/gen/go/argus-labs/argus/grpc/go/v1/sidecarv1grpc"
)

type NakamaHookAction func(client g1.NakamaClient, ctx sdk.Context, msg core.Message, receipt *types.Receipt) error

// NakamaHook is a hook that can react to EVM events with Nakama calls.
type NakamaHook struct {
	EventSignature common.Hash
	Action         NakamaHookAction
}

func NewNakamaHook(eventSignature common.Hash, action NakamaHookAction) NakamaHook {
	return NakamaHook{eventSignature, action}
}
