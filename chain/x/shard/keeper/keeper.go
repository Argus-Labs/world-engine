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
	for _, txs := range genesis.Transactions {
		namespace := txs.Namespace
		for _, tx := range txs.Txs {
			err := k.saveTransaction(ctx, namespace, tx)
			if err != nil {
				panic(err)
			}
		}
	}
}

func (k *Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState {
	allTxs := make([]*types.Transactions, 0)
	k.iterateNamespaces(ctx, func(ns string) bool {
		txs := &types.Transactions{Namespace: ns}
		k.iterateTransactions(ctx, nil, nil, ns, func(_ []byte, tx []byte) bool {
			txs.Txs = append(txs.Txs, tx)
			return true
		})
		allTxs = append(allTxs, txs)
		return true
	})
	return &types.GenesisState{Transactions: allTxs}
}
