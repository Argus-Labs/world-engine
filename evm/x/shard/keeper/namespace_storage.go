package keeper

import (
	"cosmossdk.io/store/prefix"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	namespaceStorePrefix = []byte("nss")
)

// nameSpaceStore retrieves the store for storing namespaces.
func (k *Keeper) nameSpaceStore(ctx sdk.Context) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, namespaceStorePrefix)
}

// iterateNamespaces iterates over all namespaces, calling fn for each batch in the store.
// if fn returns false, the iteration stops. if fn returns true, the iteration continues.
func (k *Keeper) iterateNamespaces(ctx sdk.Context, cb func(ns string) bool) {
	store := k.nameSpaceStore(ctx)
	it := store.Iterator(nil, nil)
	for ; it.Valid(); it.Next() {
		if keepGoing := cb(string(it.Key())); !keepGoing {
			break
		}
	}
}

// saveNamespace saves a namespace to the store.
func (k *Keeper) saveNamespace(ctx sdk.Context, ns string) {
	store := k.nameSpaceStore(ctx)
	if store.Has([]byte(ns)) {
		return
	}
	store.Set([]byte(ns), []byte{})
}
