package keeper

import (
	"cosmossdk.io/core/store"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"pkg.world.dev/world-engine/evm/x/shard/types"
)

type Keeper struct {
	storeService store.KVStoreService
	auth         string
}

func NewKeeper(ss store.KVStoreService, auth string) *Keeper {
	if auth == "" {
		panic("shard keeper: no auth address")
	}
	k := &Keeper{storeService: ss, auth: auth}
	return k
}

func (k *Keeper) InitGenesis(ctx sdk.Context, genesis *types.GenesisState) {
	for _, nstx := range genesis.NamespaceTransactions {
		namespace := nstx.Namespace
		for _, epochTxs := range nstx.Epochs {
			err := k.saveTransactions(ctx, namespace, epochTxs)
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
			Epochs:    nil,
		}
		k.iterateTransactions(ctx, nil, nil, ns, func(e *types.Epoch) bool {
			nstxs.Epochs = append(nstxs.Epochs, e)
			return true
		})
		res.NamespaceTransactions = append(res.NamespaceTransactions, nstxs)
		return true
	})
	return res
}

func (k *Keeper) AuthorityAddress() sdk.AccAddress {
	return authtypes.NewModuleAddress(k.auth)
}
