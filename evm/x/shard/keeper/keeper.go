package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"log"
	"pkg.world.dev/world-engine/evm/sequencer"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	"strings"

	"cosmossdk.io/core/store"
)

type Keeper struct {
	storeService store.KVStoreService
	auth         string
}

func NewKeeper(ss store.KVStoreService, auth string) *Keeper {
	if auth == "" {
		auth = authtypes.NewModuleAddress(sequencer.Name).String()
		if strings.Contains(auth, "cosmos") {
			log.Fatal("address had 'cosmos' bech32 prefix, should be 'world'")
		}
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
