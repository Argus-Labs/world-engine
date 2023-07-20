package keeper

import (
	"cosmossdk.io/store/prefix"
	"encoding/binary"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

/*
Storage Layout:
p = store prefix
k = key
v = value
		Namespaces(Singleton array): 		p<nsi> : k<namespace> -> v<>
		Transactions(Incremental mapping: 	p<world_namespace> : k<transaction_index> -> v<tx>
		Transaction Indexes: 				p<nsi> : k<world_namespace> -> v<transaction_index>
*/

const (
	uint64Size = 8
)

var (
	namespaceStorePrefix      = []byte("nss")
	namespaceIndexStorePrefix = []byte("nsi")
)

// transactionStore retrieves the store for storing transactions from a given world.
func (k *Keeper) transactionStore(ctx sdk.Context, worldNamespace string) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, []byte(worldNamespace))
}

// nameSpaceStore retrieves the store for storing namespaces.
func (k *Keeper) nameSpaceStore(ctx sdk.Context) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, namespaceStorePrefix)
}

func (k *Keeper) namespaceIndexStore(ctx sdk.Context) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, namespaceIndexStorePrefix)
}

func (k *Keeper) setNamespaceIndex(store prefix.Store, ns string, i uint64) {
	value := k.bytesForUint(i)
	store.Set([]byte(ns), value)
}

func (k *Keeper) incrementNamespaceIndex(store prefix.Store, ns string) uint64 {
	bz := store.Get([]byte(ns))
	var idx uint64 = 1
	// if the chain has no index saved, the get will return nil. in that case, we want to use the default value
	// set above. else, we want to convert the bytes given to uint64.
	if bz != nil {
		idx = k.uint64ForBytes(bz)
	}
	k.setNamespaceIndex(store, ns, idx+1)
	return idx
}

func (k *Keeper) getTransactionKey(ctx sdk.Context, ns string) []byte {
	store := k.namespaceIndexStore(ctx)
	idx := k.incrementNamespaceIndex(store, ns)
	return k.bytesForUint(idx)
}

//// iterateBatches iterates over all batches, calling fn for each batch in the store.
//// if fn returns false, the iteration stops. if fn returns true, the iteration continues.
//// start and end indicate the range of the iteration. Leaving both as nil will iterate over ALL batches.
//// supplying only a start value will iterate from that point til the end.
//func (k *Keeper) iterateBatches(
//	ctx sdk.Context,
//	start, end []byte,
//	ns string,
//	cb func(tick uint64, batch []byte) bool) {
//	store := k.batchStore(ctx, ns)
//	it := store.Iterator(start, end)
//	for ; it.Valid(); it.Next() {
//		tick := k.uint64ForBytes(it.Key())
//		batch := it.Value()
//		if keepGoing := cb(tick, batch); !keepGoing {
//			break
//		}
//	}
//}

func (k *Keeper) iterateTransactions(
	ctx sdk.Context,
	start, end []byte,
	ns string,
	cb func(key []byte, tx []byte) bool) {
	store := k.transactionStore(ctx, ns)
	it := store.Iterator(start, end)
	for ; it.Valid(); it.Next() {
		key := it.Key()
		tx := it.Value()
		// if callback returns false, we stop.
		if !cb(key, tx) {
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

func (k *Keeper) saveTransaction(ctx sdk.Context, ns string, tx []byte) error {
	k.saveNamespace(ctx, ns)
	store := k.transactionStore(ctx, ns)
	key := k.getTransactionKey(ctx, ns)
	store.Set(key, tx)
	return nil
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
