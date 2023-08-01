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

// transactions are keyed via epochs.
func (k *Keeper) getTransactionKey(epoch uint64) []byte {
	buf := make([]byte, uint64Size)
	binary.BigEndian.PutUint64(buf, epoch)
	return buf
}

func (k *Keeper) iterateTransactions(
	ctx sdk.Context,
	start, end []byte,
	ns string,
	cb func(e *types.Epoch) bool) {
	store := k.transactionStore(ctx, ns)
	it := store.Iterator(start, end)
	for ; it.Valid(); it.Next() {
		epochBz := it.Value()
		epoch := new(types.Epoch)
		err := epoch.Unmarshal(epochBz)
		if err != nil {
			// this shouldn't ever happen, so lets just panic if it somehow does.
			panic(fmt.Errorf("error while unmarshalling transaction bytes into %T: %w", epoch, err))
		}
		// if callback returns false, we stop.
		if !cb(epoch) {
			break
		}
	}
}

func (k *Keeper) saveTransactions(ctx sdk.Context, ns string, e *types.Epoch) error {
	k.saveNamespace(ctx, ns)
	store := k.transactionStore(ctx, ns)
	key := k.getTransactionKey(e.Epoch)
	bz, err := e.Marshal()
	if err != nil {
		return err
	}
	store.Set(key, bz)
	return nil
}
