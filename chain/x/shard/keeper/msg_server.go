package keeper

import (
	"context"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"pkg.world.dev/world-engine/chain/x/shard/types"
)

var _ types.MsgServer = &Keeper{}

func (k *Keeper) SubmitShardTx(ctx context.Context, msg *types.SubmitShardTxRequest,
) (*types.SubmitShardTxResponse, error) {
	if msg.Sender != k.auth {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("SubmitShardTx is a system function and cannot be called "+
			"externally. expected %s, got %s", k.auth, msg.Sender)
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	err := k.saveTransactions(sdkCtx, msg.Namespace, &types.Epoch{
		Epoch: msg.Epoch,
		Txs:   msg.Txs,
	})
	if err != nil {
		return nil, err
	}

	return &types.SubmitShardTxResponse{}, nil
}
