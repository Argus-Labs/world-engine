package keeper

import (
	"encoding/json"

	abci "github.com/cometbft/cometbft/abci/types"

	"cosmossdk.io/orm/model/ormdb"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/argus-labs/world-engine/chain/x/router/storage"

	api "github.com/argus-labs/world-engine/chain/api/router/v1"

	"cosmossdk.io/core/genesis"
)

type Keeper struct {
	// store is the storage for the router module.
	store api.StateStore

	db ormdb.ModuleDB
	// authority is the bech32 address that is allowed to execute governance proposals.
	authority string
}

func NewKeeper(ss api.StateStore, auth string) *Keeper {
	db, err := ormdb.NewModuleDB(&storage.ModuleSchema, ormdb.ModuleDBOptions{})
	if err != nil {
		panic(err)
	}
	return &Keeper{
		store:     ss,
		db:        db,
		authority: auth,
	}
}

func (k *Keeper) InitGenesis(ctx sdk.Context, _ codec.JSONCodec, data json.RawMessage) ([]abci.ValidatorUpdate, error) {
	source, err := genesis.SourceFromRawJSON(data)
	if err != nil {
		return nil, err
	}

	err = k.db.GenesisHandler().ValidateGenesis(source)
	if err != nil {
		return nil, err
	}

	err = k.db.GenesisHandler().InitGenesis(ctx, source)

	return []abci.ValidatorUpdate{}, nil
}

func (k *Keeper) ExportGenesis(ctx sdk.Context, _ codec.JSONCodec) (json.RawMessage, error) {
	target := genesis.RawJSONTarget{}
	err := k.db.GenesisHandler().ExportGenesis(ctx, target.Target())
	if err != nil {
		return nil, err
	}
	return target.JSON()
}
