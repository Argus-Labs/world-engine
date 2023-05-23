package keeper

import (
	"context"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/argus-labs/world-engine/chain/x/router/types"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ types.MsgServiceServer = &Keeper{}

func (k *Keeper) UpdateNamespace(ctx context.Context, request *types.UpdateNamespaceRequest) (*types.UpdateNamespaceResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if k.authority != request.Authority {
		return nil, sdkerrors.ErrUnauthorized.Wrapf("%s is not allowed to update namespaces, expected %s", request.Authority, k.authority)
	}
	store := sdkCtx.KVStore(k.storeKey)
	pStore := prefix.NewStore(store, NamespaceKey)

	pStore.Set([]byte(request.ShardName), []byte(request.ShardAddress))
	return &types.UpdateNamespaceResponse{}, nil
}
