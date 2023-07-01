package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/argus-labs/world-engine/chain/x/shard/types"

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

func (k *Keeper) InitGenesis(ctx sdk.Context, genesis *types.GenesisState) {
	for _, b := range genesis.Batches {
		k.saveBatch(ctx, b)
	}
}

func (k *Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	batches := make([]*types.TransactionBatch, 0)
	k.iterateNamespaces(ctx, func(ns string) bool {
		k.iterateBatches(ctx, ns, func(tick uint64, batch []byte) bool {
			batches = append(batches, &types.TransactionBatch{
				Namespace: ns,
				Tick:      tick,
				Batch:     batch,
			})
			return true
		})
		return true
	})
	return &types.GenesisState{Batches: batches}
}
