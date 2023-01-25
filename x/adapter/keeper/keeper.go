package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "github.com/cosmos/cosmos-sdk/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/argus-labs/argus/x/adapter/types/v1"
)

var (
	_ v1.MsgServer   = Keeper{}
	_ v1.QueryServer = Keeper{}
)

type Keeper struct {
	cdc codec.Codec

	storeKey storetypes.StoreKey

	moduleAddr string
}

var AllowContractCreationPrefix = []byte{0x1}

func NewKeeper(cdc codec.Codec, sk storetypes.StoreKey, moduleAddr string) Keeper {
	return Keeper{cdc, sk, moduleAddr}
}

// TX SERVICE

func (k Keeper) ClaimQuestReward(ctx context.Context, msg *v1.MsgClaimQuestReward) (*v1.MsgClaimQuestRewardResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.Logger().Info("ClaimQuestReward Called: %v", msg)
	return &v1.MsgClaimQuestRewardResponse{Reward_ID: "foobar"}, nil
}

func (k Keeper) AllowContractCreation(ctx context.Context, msg *v1.MsgAllowContractCreation) (*v1.MsgAllowContractCreationResponse, error) {
	if msg.Sender != k.moduleAddr {
		return nil, sdkerrors.ErrUnauthorized
	}
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	store := sdkCtx.KVStore(k.storeKey)
	key := k.makeContractCreatorStoreKey(msg.Addr)
	store.Set(key, []byte{0x0})
	return &v1.MsgAllowContractCreationResponse{}, nil
}

func (k Keeper) CheckAddr(sdkCtx sdk.Context, addr string) bool {
	store := sdkCtx.KVStore(k.storeKey)
	key := k.makeContractCreatorStoreKey(addr)
	v := store.Get(key)
	return v != nil
}

// QUERY SERVICE

func (k Keeper) AllowedContractCreator(ctx context.Context, query *v1.QueryAllowedContractCreator) (*v1.QueryAllowedContractCreatorResponse, error) {
	return &v1.QueryAllowedContractCreatorResponse{Allowed: k.CheckAddr(sdk.UnwrapSDKContext(ctx), query.Addr)}, nil
}

// UTILITIES

func (k Keeper) makeContractCreatorStoreKey(addr string) []byte {
	return append(AllowContractCreationPrefix, []byte(addr)...)
}
