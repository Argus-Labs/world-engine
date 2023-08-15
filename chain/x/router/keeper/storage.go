package keeper

import (
	"fmt"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"pkg.world.dev/world-engine/chain/x/router/types"
)

var (
	namespacePrefix = []byte("ns")
)

func (k *Keeper) getNamespaceStore(ctx sdk.Context) prefix.Store {
	return prefix.NewStore(ctx.KVStore(k.storeKey), namespacePrefix)
}

func (k *Keeper) getAddressForNamespace(ctx sdk.Context, ns string) (string, error) {
	store := k.getNamespaceStore(ctx)
	addr := store.Get([]byte(ns))
	if addr == nil {
		return "", fmt.Errorf("address for name %s does not exist", ns)
	}
	return string(addr), nil
}

func (k *Keeper) setNamespace(ctx sdk.Context, ns *types.Namespace) {
	store := k.getNamespaceStore(ctx)
	store.Set([]byte(ns.ShardName), []byte(ns.ShardAddress))
}

func (k *Keeper) getAllNamespaces(ctx sdk.Context) []*types.Namespace {
	store := k.getNamespaceStore(ctx)
	it := store.Iterator(nil, nil)
	namespaces := make([]*types.Namespace, 0)
	for ; it.Valid(); it.Next() {
		namespaces = append(namespaces, &types.Namespace{
			ShardName:    string(it.Key()),
			ShardAddress: string(it.Value()),
		})
	}
	return namespaces
}
