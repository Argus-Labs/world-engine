package keeper

import (
	"cosmossdk.io/store/prefix"
	"encoding/binary"
	"fmt"
	"github.com/argus-labs/world-engine/chain/x/shard/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

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
