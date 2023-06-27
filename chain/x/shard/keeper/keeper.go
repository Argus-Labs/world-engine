package keeper

import (
	shardTypes "github.com/argus-labs/world-engine/chain/x/shard/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"cosmossdk.io/core/store"
)

type Keeper struct {
	storeService store.KVStoreService
	auth         string
}

func NewKeeper(ss store.KVStoreService, auth string) *Keeper {
	k := &Keeper{storeService: ss, auth: auth}
	return k
}

func (k *Keeper) InitGenesis(ctx sdk.Context, genesis *shardTypes.GenesisState) {
	for _, batch := range genesis.Batches {
		k.saveBatch(ctx, batch)
	}
	k.saveIndex(ctx, genesis.Index)
}

func (k *Keeper) ExportGenesis(ctx sdk.Context) *shardTypes.GenesisState {
	batches := make([][]byte, 0)
	k.iterateBatches(ctx, func(batch []byte) bool {
		batches = append(batches, batch)
		return true
	})
	idx := k.getCurrentIndex(ctx)
	return &shardTypes.GenesisState{
		Batches: batches,
		Index:   idx,
	}
}
