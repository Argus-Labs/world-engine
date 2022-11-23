package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/argus-labs/argus/x/adapter/types"
)

var _ types.MsgServer = Keeper{}

type Keeper struct {
	cdc codec.Codec

	storeKey storetypes.StoreKey
}

func NewKeeper(cdc codec.Codec, sk storetypes.StoreKey) Keeper {
	return Keeper{cdc, sk}
}

func (k Keeper) ClaimQuestReward(ctx context.Context, msg *types.MsgClaimQuestReward) (*types.MsgClaimQuestRewardResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.Logger().Info("ClaimQuestReward Called: %v", msg)
	return &types.MsgClaimQuestRewardResponse{RewardId: "foobar"}, nil
}
