package keeper

import (
	"encoding/binary"
	"github.com/cosmos/cosmos-sdk/runtime"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	uint64Size = 8
)

var (
	batchStoragePrefix      = []byte("batch")
	batchStorageIndexPrefix = []byte("idx")

	idxKey = []byte("i")
)

// iterateBatches iterates over all batches, calling fn for each batch in the store.
// if fn returns false, the iteration stops. if fn returns true, the iteration continues.
func (k *Keeper) iterateBatches(ctx sdk.Context, fn func(batch []byte) bool) {
	store := k.getBatchStore(ctx)
	it := store.Iterator(nil, nil)
	for ; it.Valid(); it.Next() {
		batch := it.Value()
		keepGoing := fn(batch)
		if !keepGoing {
			break
		}
	}
}

func (k *Keeper) saveBatch(ctx sdk.Context, batch []byte) {
	store := k.getBatchStore(ctx)
	key := k.getNextBatchIndexBytes(ctx)
	store.Set(key, batch)
}

func (k *Keeper) getBatchStore(ctx sdk.Context) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, batchStoragePrefix)
}

func (k *Keeper) getBatchIndexStore(ctx sdk.Context) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, batchStorageIndexPrefix)
}

func (k *Keeper) getNextBatchIndexBytes(ctx sdk.Context) []byte {
	store := k.getBatchIndexStore(ctx)
	bz := store.Get(idxKey)
	// the bytes can be nil if this is the first time we are accessing this value from the store.
	if bz == nil {
		bz = make([]byte, uint64Size)
		binary.BigEndian.PutUint64(bz, 0)
	}
	idx := k.indexFromBytes(bz)

	nextIdx := idx + 1
	store.Set(idxKey, k.bytesFromIndex(nextIdx))

	return bz
}

func (k *Keeper) saveIndex(ctx sdk.Context, idx uint64) {
	store := k.getBatchIndexStore(ctx)
	store.Set(idxKey, k.bytesFromIndex(idx))
}

func (k *Keeper) getCurrentIndex(ctx sdk.Context) uint64 {
	store := k.getBatchIndexStore(ctx)
	bz := store.Get(idxKey)
	return k.indexFromBytes(bz)
}

func (k *Keeper) indexFromBytes(bz []byte) uint64 {
	return binary.BigEndian.Uint64(bz)
}

func (k *Keeper) bytesFromIndex(idx uint64) []byte {
	buf := make([]byte, uint64Size)
	binary.BigEndian.PutUint64(buf, idx)
	return buf
}
