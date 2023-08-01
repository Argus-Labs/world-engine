package keeper

import (
	"context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

var _ types.MsgServer = &Keeper{}

func (k *Keeper) SubmitShardTx(ctx context.Context, msg *types.SubmitShardTxRequest,
) (*types.SubmitShardTxResponse, error) {
	if msg.Sender != k.auth {
		return nil, sdkerrors.ErrUnauthorized.Wrap("SubmitShardTx is a system function and cannot be called " +
			"externally.")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	err := k.saveTransactions(sdkCtx, msg.Namespace, &types.Tick{
		Tick: msg.Tick,
		Txs:  msg.Txs,
	})
	if err != nil {
		return nil, err
	}

	return &types.SubmitShardTxResponse{}, nil
}
