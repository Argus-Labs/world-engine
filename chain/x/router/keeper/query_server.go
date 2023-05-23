package keeper

import (
	"context"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/argus-labs/world-engine/chain/x/router/types"
)

func (k *Keeper) Namespaces(ctx context.Context, request *types.NamespacesRequest) (*types.NamespacesResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	store := sdkCtx.KVStore(k.storeKey)
	pStore := prefix.NewStore(store, NamespaceKey)
	it := pStore.Iterator(nil, nil)
	nameSpaces := make([]*types.Namespace, 0, 5)
	for it.Valid() {
		key, val := it.Key(), it.Value()
		nameSpaces = append(nameSpaces, &types.Namespace{ShardName: string(key), ShardAddress: string(val)})
		it.Next()
	}
	return &types.NamespacesResponse{Namespaces: nameSpaces}, nil
}
