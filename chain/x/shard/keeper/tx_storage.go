package keeper

import (
	"cosmossdk.io/store/prefix"
	"encoding/binary"
	"fmt"
	"github.com/argus-labs/world-engine/chain/x/shard/types"
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

// transactionStore retrieves the store for storing transactions from a given world.
func (k *Keeper) transactionStore(ctx sdk.Context, worldNamespace string) prefix.Store {
	store := runtime.KVStoreAdapter(k.storeService.OpenKVStore(ctx))
	return prefix.NewStore(store, []byte(worldNamespace))
}

// transactions are keyed via ticks.
func (k *Keeper) getTransactionKey(tick uint64) []byte {
	buf := make([]byte, uint64Size)
	binary.BigEndian.PutUint64(buf, tick)
	return buf
}

func (k *Keeper) extractTransactionKeyValue(key []byte) uint64 {
	return binary.BigEndian.Uint64(key)
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
	cb func(tick uint64, txs *types.Transactions) bool) {
	store := k.transactionStore(ctx, ns)
	it := store.Iterator(start, end)
	for ; it.Valid(); it.Next() {
		key := it.Key()
		tick := k.extractTransactionKeyValue(key)
		bzTxs := it.Value()
		txs := new(types.Transactions)
		err := txs.Unmarshal(bzTxs)
		if err != nil {
			// this shouldn't ever happen, so lets just panic if it somehow does.
			panic(fmt.Errorf("error while unmarshalling transaction bytes into %T: %w", txs, err))
		}
		// if callback returns false, we stop.
		if !cb(tick, txs) {
			break
		}
	}
}

func (k *Keeper) saveTransactions(ctx sdk.Context, ns string, tick uint64, txs *types.Transactions) error {
	k.saveNamespace(ctx, ns)
	store := k.transactionStore(ctx, ns)
	key := k.getTransactionKey(tick)
	bz, err := txs.Marshal()
	if err != nil {
		return err
	}
	store.Set(key, bz)
	return nil
}
