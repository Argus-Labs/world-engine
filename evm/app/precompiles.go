// SPDX-License-Identifier: BUSL-1.1
//
// Copyright (C) 2023, Berachain Foundation. All rights reserved.
// Use of this software is govered by the Business Source License included
// in the LICENSE file of this repository and at www.mariadb.com/bsl11.
//
// ANY USE OF THE LICENSED WORK IN VIOLATION OF THIS LICENSE WILL AUTOMATICALLY
// TERMINATE YOUR RIGHTS UNDER THIS LICENSE FOR THE CURRENT AND ALL OTHER
// VERSIONS OF THE LICENSED WORK.
//
// THIS LICENSE DOES NOT GRANT YOU ANY RIGHT IN ANY TRADEMARK OR LOGO OF
// LICENSOR OR ITS AFFILIATES (PROVIDED THAT YOU MAY USE A TRADEMARK OR LOGO OF
// LICENSOR AS EXPRESSLY REQUIRED BY THIS LICENSE).
//
// TO THE EXTENT PERMITTED BY APPLICABLE LAW, THE LICENSED WORK IS PROVIDED ON
// AN “AS IS” BASIS. LICENSOR HEREBY DISCLAIMS ALL WARRANTIES AND CONDITIONS,
// EXPRESS OR IMPLIED, INCLUDING (WITHOUT LIMITATION) WARRANTIES OF
// MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, NON-INFRINGEMENT, AND
// TITLE.

package app

import (
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"

	bankprecompile "pkg.berachain.dev/polaris/cosmos/precompile/bank"
	distrprecompile "pkg.berachain.dev/polaris/cosmos/precompile/distribution"
	govprecompile "pkg.berachain.dev/polaris/cosmos/precompile/governance"
	stakingprecompile "pkg.berachain.dev/polaris/cosmos/precompile/staking"
	ethprecompile "pkg.berachain.dev/polaris/eth/core/precompile"

	"pkg.world.dev/world-engine/evm/precompile/router"
)

// PrecompilesToInject returns a function that provides the initialization of the standard
// set of precompiles.
func PrecompilesToInject(app *App, customPcs ...ethprecompile.Registrable) func() *ethprecompile.Injector {
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
