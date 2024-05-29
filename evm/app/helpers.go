package app

import (
	evmconfig "github.com/berachain/polaris/cosmos/config"
	bankprecompile "github.com/berachain/polaris/cosmos/precompile/bank"
	distrprecompile "github.com/berachain/polaris/cosmos/precompile/distribution"
	govprecompile "github.com/berachain/polaris/cosmos/precompile/governance"
	stakingprecompile "github.com/berachain/polaris/cosmos/precompile/staking"
	ethprecompile "github.com/berachain/polaris/eth/core/precompile"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"

	"pkg.world.dev/world-engine/evm/precompile/router"
)

// PrecompilesToInject returns a function that provides the initialization of the standard
// set of precompiles.
func PrecompilesToInject(
	app *App,
	customPcs ...ethprecompile.Registrable,
) func() *ethprecompile.Injector {
	return func() *ethprecompile.Injector {
		// Create the precompile injector with the standard precompiles.
		pcs := ethprecompile.NewPrecompiles([]ethprecompile.Registrable{
			bankprecompile.NewPrecompileContract(
				app.AccountKeeper,
				bankkeeper.NewMsgServerImpl(app.BankKeeper),
				app.BankKeeper,
			),
			distrprecompile.NewPrecompileContract(
				app.AccountKeeper,
				app.StakingKeeper,
				distrkeeper.NewMsgServerImpl(app.DistrKeeper),
				distrkeeper.NewQuerier(app.DistrKeeper),
			),
			govprecompile.NewPrecompileContract(
				app.AccountKeeper,
				govkeeper.NewMsgServerImpl(app.GovKeeper),
				govkeeper.NewQueryServer(app.GovKeeper),
				app.interfaceRegistry,
			),
			stakingprecompile.NewPrecompileContract(app.AccountKeeper, app.StakingKeeper),
			router.NewPrecompileContract(app.Router),
		}...)

		// Add the custom precompiles to the injector.
		for _, pc := range customPcs {
			pcs.AddPrecompile(pc)
		}
		return pcs
	}
}

func QueryContextFn(app *App) func() func(height int64, prove bool) (sdk.Context, error) {
	return func() func(height int64, prove bool) (sdk.Context, error) {
		return app.BaseApp.CreateQueryContext
	}
}

// PolarisConfigFn returns a function that provides the initialization of the standard
// set of precompiles.
func PolarisConfigFn(cfg *evmconfig.Config) func() *evmconfig.Config {
	return func() *evmconfig.Config {
		return cfg
	}
}
