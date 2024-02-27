package namespace

import (
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/depinject"
	storetypes "cosmossdk.io/store/types"
	v1 "pkg.world.dev/world-engine/evm/api/namespace/module/v1"
	"pkg.world.dev/world-engine/evm/x/namespace/keeper"
)

//nolint:gochecknoinits // GRRRR fix later.
func init() {
	appmodule.Register(&v1.Module{}, appmodule.Provide(ProvideModule))
}

// DepInjectInput is the input for the dep inject framework.
type DepInjectInput struct {
	depinject.In

	ModuleKey depinject.OwnModuleKey
	Config    *v1.Module
	StoreKey  *storetypes.KVStoreKey
}

// DepInjectOutput is the output for the dep inject framework.
type DepInjectOutput struct {
	depinject.Out

	Keeper *keeper.Keeper
	Module appmodule.AppModule
}

// ProvideModule is a function that provides the module to the application.
func ProvideModule(in DepInjectInput) DepInjectOutput {

	k := keeper.NewKeeper(
		in.StoreKey,
		"",
	)

	m := NewAppModule(k)

	return DepInjectOutput{Keeper: k, Module: m}
}
