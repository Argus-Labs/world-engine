package router

import (
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/depinject"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	v1 "github.com/argus-labs/world-engine/chain/api/router/module/v1"
	api "github.com/argus-labs/world-engine/chain/api/router/v1"

	"github.com/argus-labs/world-engine/chain/x/router/keeper"
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
	Store     api.StateStore
	AppOpts   servertypes.AppOptions
}

// DepInjectOutput is the output for the dep inject framework.
type DepInjectOutput struct {
	depinject.Out

	Keeper *keeper.Keeper
	Module appmodule.AppModule
}

// ProvideModule is a function that provides the module to the application.
func ProvideModule(in DepInjectInput) DepInjectOutput {
	// Default to governance authority if not provided
	authority := authtypes.NewModuleAddress(govtypes.ModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}

	k := keeper.NewKeeper(
		in.Store,
		authority.String(),
	)

	m := NewAppModule(k)

	return DepInjectOutput{Keeper: k, Module: m}
}
