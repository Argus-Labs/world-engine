package keeper

import (
	api "github.com/argus-labs/world-engine/chain/api/router/v1"
)

type Keeper struct {
	// store is the storage for the router module.
	store api.StateStore
	// authority is the bech32 address that is allowed to execute governance proposals.
	authority string
}

func NewKeeper(ss api.StateStore, auth string) *Keeper {
	return &Keeper{
		store:     ss,
		authority: auth,
	}
}
