package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

var _ types.MsgServer = &Keeper{}

func (k *Keeper) SubmitBatch(ctx context.Context, msg *types.SubmitBatchRequest) (*types.SubmitBatchResponse, error) {
	if msg.Sender != k.auth {
		return nil, sdkerrors.ErrUnauthorized.Wrap("this function cannot be used by EOAs. the transaction must be " +
			"initialized internally")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	k.saveBatch(sdkCtx, msg.Batch)
	return &types.SubmitBatchResponse{}, nil
}
