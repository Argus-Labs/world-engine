package keeper

import (
	"cosmossdk.io/store/types"
	shardTypes "github.com/argus-labs/world-engine/chain/x/shard/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

type Keeper struct {
	storeKey *types.KVStoreKey
	auth     string
}

func NewKeeper(sk *types.KVStoreKey, auth string) *Keeper {
	k := &Keeper{storeKey: sk, auth: auth}
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
