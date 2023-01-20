package keeper

import (
	"context"
	"fmt"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/argus-labs/argus/x/adapter/types/v1"
)

var _ v1.MsgServer = Keeper{}
var _ v1.QueryServer = Keeper{}

type Keeper struct {
	cdc        codec.Codec
	ModuleAddr string
	storeKey   storetypes.StoreKey

	prefixDAKey []byte
}

func (k Keeper) GameState(ctx context.Context, _ *v1.QueryGameStateRequest) (*v1.QueryGameStateResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	kvStore := sdkCtx.KVStore(k.storeKey)
	bz := kvStore.Get(k.prefixDAKey)
	gs := v1.MsgUpdateGameState{}
	err := gs.Unmarshal(bz)
	if err != nil {
		return nil, err
	}
	return &v1.QueryGameStateResponse{NumPlanets: gs.NumPlanets}, nil
}

func (k Keeper) UpdateGameState(ctx context.Context, state *v1.MsgUpdateGameState) (*v1.MsgUpdateGameStateResponse, error) {
	if state.Sender != k.ModuleAddr {
		return nil, fmt.Errorf("only the adapter module can call this method")
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	kvStore := sdkCtx.KVStore(k.storeKey)
	bz, err := state.Marshal()
	if err != nil {
		return nil, err
	}
	kvStore.Set(k.prefixDAKey, bz)
	return &v1.MsgUpdateGameStateResponse{}, nil
}

func NewKeeper(cdc codec.Codec, sk storetypes.StoreKey, moduleAddr string, prefixDAKey []byte) Keeper {
	return Keeper{cdc, moduleAddr, sk, prefixDAKey}
}

func (k Keeper) ClaimQuestReward(ctx context.Context, msg *v1.MsgClaimQuestReward) (*v1.MsgClaimQuestRewardResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.Logger().Info("ClaimQuestReward Called: %v", msg)
	return &v1.MsgClaimQuestRewardResponse{Reward_ID: "foobar"}, nil
}
