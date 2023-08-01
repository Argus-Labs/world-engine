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
	for _, nstx := range genesis.NamespaceTransactions {
		namespace := nstx.Namespace
		for _, tickedTx := range nstx.Ticks {
			err := k.saveTransactions(ctx, namespace, tickedTx.Tick, tickedTx.Txs)
			if err != nil {
				panic(err)
			}
		}
	}
}

func (k *Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	res := new(types.GenesisState)
	k.iterateNamespaces(ctx, func(ns string) bool {
		nstxs := &types.NamespaceTransactions{
			Namespace: ns,
			Ticks:     nil,
		}
		k.iterateTransactions(ctx, nil, nil, ns, func(tick uint64, txs *types.Transactions) bool {
			nstxs.Ticks = append(nstxs.Ticks, &types.Tick{
				Tick: tick,
				Txs:  txs,
			})
			return true
		})
		res.NamespaceTransactions = append(res.NamespaceTransactions, nstxs)
		return true
	})
	return res
}
