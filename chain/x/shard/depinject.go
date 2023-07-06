package shard

import (
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	modulev1 "github.com/argus-labs/world-engine/chain/api/shard/module/v1"
	"github.com/argus-labs/world-engine/chain/x/shard/keeper"
)

//nolint:gochecknoinits // GRRRR fix later.
func init() {
	appmodule.Register(&modulev1.Module{}, appmodule.Provide(ProvideModule))
}

// DepInjectInput is the input for the dep inject framework.
type DepInjectInput struct {
	depinject.In

	ModuleKey    depinject.OwnModuleKey
	Config       *modulev1.Module
	StoreService store.KVStoreService
}

// DepInjectOutput is the output for the dep inject framework.
type DepInjectOutput struct {
	depinject.Out

	Keeper *keeper.Keeper
	Module appmodule.AppModule
}

func ProvideModule(in DepInjectInput) DepInjectOutput {
	k := keeper.NewKeeper(in.StoreService, in.Config.Authority)
	m := NewAppModule(k)
	return DepInjectOutput{
		Keeper: k,
		Module: m,
	}
}
