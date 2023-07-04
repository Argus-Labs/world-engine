package keeper

import (
	"encoding/binary"

	"github.com/cosmos/cosmos-sdk/runtime"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/argus-labs/world-engine/chain/x/shard/types"
)

/*
Storage Layout:
		Namespaces : <namespace> -> {}
		Batches    : <namespace>-<tick> -> <batch_bytes>
*/

const (
	uint64Size = 8
)

var (
	namespaceStorePrefix = []byte("nss")
)

// batchStore retrieves the store for storing batches within a given namespace.
func (k *Keeper) batchStore(ctx sdk.Context, ns string) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, []byte(ns))
}

// nameSpaceStore retrieves the store for storing namespaces.
func (k *Keeper) nameSpaceStore(ctx sdk.Context) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, namespaceStorePrefix)
}

// iterateBatches iterates over all batches, calling fn for each batch in the store.
// if fn returns false, the iteration stops. if fn returns true, the iteration continues.
func (k *Keeper) iterateBatches(ctx sdk.Context, start, end []byte, ns string, cb func(tick uint64, batch []byte) bool) {
	store := k.batchStore(ctx, ns)
	it := store.Iterator(start, end)
	for ; it.Valid(); it.Next() {
		tick := k.uint64ForBytes(it.Key())
		batch := it.Value()
		if keepGoing := cb(tick, batch); !keepGoing {
			break
		}
	}
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

// saveBatch saves a batch of transaction data.
func (k *Keeper) saveBatch(ctx sdk.Context, req *types.TransactionBatch) {
	k.saveNamespace(ctx, req.Namespace)
	store := k.batchStore(ctx, req.Namespace)
	key := k.bytesForUint(req.Tick)
	store.Set(key, req.Batch)
}

// saveNamespace saves a namespace to the store.
func (k *Keeper) saveNamespace(ctx sdk.Context, ns string) {
	store := k.nameSpaceStore(ctx)
	if store.Has([]byte(ns)) {
		return
	}
	store.Set([]byte(ns), []byte{})
}

// bytesForUint converts uint to big endian encoded bytes.
func (k *Keeper) bytesForUint(u uint64) []byte {
	bz := make([]byte, uint64Size)
	binary.BigEndian.PutUint64(bz, u)
	return bz
}

// uint64ForBytes converts big endian encoded bytes to uint64.
func (k *Keeper) uint64ForBytes(bz []byte) uint64 {
	return binary.BigEndian.Uint64(bz)
}
