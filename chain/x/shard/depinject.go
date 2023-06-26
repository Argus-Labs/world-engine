package shard

import (
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/depinject"
	storetypes "cosmossdk.io/store/types"

	modulev1 "github.com/argus-labs/world-engine/chain/api/router/module/v1"
	"github.com/argus-labs/world-engine/chain/x/shard/keeper"
)

func init() {
	appmodule.Register(&modulev1.Module{}, appmodule.Provide())
}

// DepInjectInput is the input for the dep inject framework.
type DepInjectInput struct {
	depinject.In

	ModuleKey depinject.OwnModuleKey
	Config    *modulev1.Module
	StoreKey  *storetypes.KVStoreKey
}

// DepInjectOutput is the output for the dep inject framework.
type DepInjectOutput struct {
	depinject.Out

	Keeper *keeper.Keeper
	Module appmodule.AppModule
}

func ProvideModule(in DepInjectInput) DepInjectOutput {
	k := keeper.NewKeeper(in.StoreKey, in.Config.Authority)
	m := NewAppModule(k)
	return DepInjectOutput{
		Keeper: k,
		Module: m,
	}
}
