package keeper

import (
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
)

type Keeper struct {
	storeKey *storetypes.KVStoreKey
	// authority is the bech32 address that is allowed to execute governance proposals.
	authority string
}

func NewKeeper(storeKey *storetypes.KVStoreKey, auth string) *Keeper {
	return &Keeper{
		storeKey:  storeKey,
		authority: auth,
	}
}

func (k *Keeper) InitGenesis(ctx sdk.Context, gen *namespacetypes.Genesis) {
	for _, ns := range gen.Namespaces {
		k.setNamespace(ctx, ns)
	}
}

func (k *Keeper) ExportGenesis(ctx sdk.Context) *namespacetypes.Genesis {
	nameSpaces := k.getAllNamespaces(ctx)
	return &namespacetypes.Genesis{Namespaces: nameSpaces}
}
