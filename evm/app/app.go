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
	"github.com/cometbft/cometbft/abci/types"
	"github.com/rotisserie/eris"
	zerolog "github.com/rs/zerolog/log"
	signinglib "pkg.berachain.dev/polaris/cosmos/lib/signing"
	"pkg.berachain.dev/polaris/cosmos/runtime/miner"
	"pkg.world.dev/world-engine/evm/sequencer"

	"io"
	"os"
	"path/filepath"

	sdk "github.com/cosmos/cosmos-sdk/types"
	evmv1alpha1 "pkg.berachain.dev/polaris/cosmos/api/polaris/evm/v1alpha1"
	evmconfig "pkg.berachain.dev/polaris/cosmos/config"

	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	evidencekeeper "cosmossdk.io/x/evidence/keeper"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	dbm "github.com/cosmos/cosmos-db"
	polarruntime "pkg.berachain.dev/polaris/cosmos/runtime"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	consensuskeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	ethcryptocodec "pkg.berachain.dev/polaris/cosmos/crypto/codec"
	evmkeeper "pkg.berachain.dev/polaris/cosmos/x/evm/keeper"

	"pkg.world.dev/world-engine/evm/router"
	namespacekeeper "pkg.world.dev/world-engine/evm/x/namespace/keeper"
	shardkeeper "pkg.world.dev/world-engine/evm/x/shard/keeper"
)

// DefaultNodeHome default home directories for the application daemon.
var DefaultNodeHome string

var (
	_ runtime.AppI            = (*App)(nil)
	_ servertypes.Application = (*App)(nil)
)

// App extends an ABCI application, but with most of its parameters exported.
// They are exported for convenience in creating helper functions, as object
// capabilities aren't needed for testing.
type App struct {
	*polarruntime.Polaris
	*runtime.App
	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	txConfig          client.TxConfig
	interfaceRegistry codectypes.InterfaceRegistry

	// keepers
	AccountKeeper         authkeeper.AccountKeeper
	BankKeeper            bankkeeper.Keeper
	StakingKeeper         *stakingkeeper.Keeper
	SlashingKeeper        slashingkeeper.Keeper
	MintKeeper            mintkeeper.Keeper
	DistrKeeper           distrkeeper.Keeper
	GovKeeper             *govkeeper.Keeper
	CrisisKeeper          *crisiskeeper.Keeper
	UpgradeKeeper         *upgradekeeper.Keeper
	EvidenceKeeper        evidencekeeper.Keeper
	ConsensusParamsKeeper consensuskeeper.Keeper

	// polaris keepers
	EVMKeeper *evmkeeper.Keeper

	// world engine keepers
	NamespaceKeeper *namespacekeeper.Keeper
	ShardKeeper     *shardkeeper.Keeper

	// plugins
	Router         router.Router
	ShardSequencer *sequencer.Sequencer
}

//nolint:gochecknoinits // from sdk.
func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	DefaultNodeHome = filepath.Join(userHomeDir, ".world-evm")
}

// NewApp returns a reference to an initialized App.
func NewApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	bech32Prefix string,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *App {
	app := &App{}
	var (
		appBuilder *runtime.AppBuilder
		// merge the Config and other configuration in one config
		appConfig = depinject.Configs(
			MakeAppConfig(bech32Prefix),
			depinject.Provide(
				signinglib.ProvideNoopGetSigners[*evmv1alpha1.WrappedEthereumTransaction],
				signinglib.ProvideNoopGetSigners[*evmv1alpha1.WrappedPayloadEnvelope],
			),
			depinject.Supply(
				appOpts,
				logger,
				PolarisConfigFn(evmconfig.MustReadConfigFromAppOpts(appOpts)),
				PrecompilesToInject(app),
				QueryContextFn(app),
			),
		)
	)

	if err := depinject.Inject(appConfig,
		&appBuilder,
		&app.appCodec,
		&app.legacyAmino,
		&app.txConfig,
		&app.interfaceRegistry,
		&app.AccountKeeper,
		&app.BankKeeper,
		&app.StakingKeeper,
		&app.SlashingKeeper,
		&app.MintKeeper,
		&app.DistrKeeper,
		&app.GovKeeper,
		&app.CrisisKeeper,
		&app.UpgradeKeeper,
		&app.EvidenceKeeper,
		&app.ConsensusParamsKeeper,
		&app.EVMKeeper,
		&app.NamespaceKeeper,
		&app.ShardKeeper,
	); err != nil {
		panic(err)
	}

	// Build the app using the app builder.
	app.App = appBuilder.Build(db, traceStore, baseAppOptions...)
	app.Polaris = polarruntime.New(
		evmconfig.MustReadConfigFromAppOpts(appOpts), app.Logger(), app.EVMKeeper.Host, nil,
	)

	app.setPlugins(logger)

	// Setup Polaris Runtime.
	if err := app.Polaris.Build(app, app.EVMKeeper, miner.DefaultAllowedMsgs, app.Router.PostBlockHook); err != nil {
		panic(err)
	}
	// register streaming services
	if err := app.RegisterStreamingServices(appOpts, app.kvStoreKeys()); err != nil {
		panic(err)
	}

	/****  Module Options ****/
	app.ModuleManager.RegisterInvariants(app.CrisisKeeper)

	ethcryptocodec.RegisterInterfaces(app.interfaceRegistry)

	app.SetPreBlocker(app.preBlocker)
	if err := app.Load(loadLatest); err != nil {
		panic(err)
	}

	// Load the last state of the polaris evm.
	if err := app.Polaris.LoadLastState(
		app.CommitMultiStore(), uint64(app.LastBlockHeight()),
	); err != nil {
		panic(err)
	}

	return app
}

func (app *App) preBlocker(ctx sdk.Context, _ *types.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
	txs, inits := app.ShardSequencer.FlushMessages()

	// first register all the game shard requests
	for _, initMsg := range inits {
		zerolog.Debug().Msgf("registering %q to %q", initMsg.Namespace.ShardName, initMsg.Namespace.ShardAddress)
		handler := app.MsgServiceRouter().Handler(initMsg)
		_, err := handler(ctx, initMsg)
		if err != nil {
			return nil, eris.Wrapf(err, "failed to register namespace %q", initMsg.Namespace.ShardName)
		}
	}

	// then sequence the game shard txs
	numTxs := len(txs)
	resPreBlock := &sdk.ResponsePreBlock{}
	if numTxs > 0 {
		zerolog.Debug().Msg("sequencing messages")
		handler := app.MsgServiceRouter().Handler(txs[0])
		for _, tx := range txs {
			_, err := handler(ctx, tx)
			if err != nil {
				zerolog.Error().Err(err).Msgf("error sequencing game shard tx")
				return resPreBlock, err
			}
		}
		app.Logger().Debug("successfully sequenced %d game shard txs", numTxs)
	}
	return resPreBlock, nil
}

// Name returns the name of the App.
func (app *App) Name() string { return app.BaseApp.Name() }

// LegacyAmino returns App's amino codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *App) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
}

// AppCodec returns App's app codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *App) AppCodec() codec.Codec {
	return app.appCodec
}

// InterfaceRegistry returns App's InterfaceRegistry.
func (app *App) InterfaceRegistry() codectypes.InterfaceRegistry {
	return app.interfaceRegistry
}

// TxConfig returns App's TxConfig.
func (app *App) TxConfig() client.TxConfig {
	return app.txConfig
}

// GetKey returns the KVStoreKey for the provided store key.
//
// NOTE: This is solely to be used for testing purposes.
func (app *App) GetKey(storeKey string) *storetypes.KVStoreKey {
	sk := app.UnsafeFindStoreKey(storeKey)
	kvStoreKey, ok := sk.(*storetypes.KVStoreKey)
	if !ok {
		return nil
	}
	return kvStoreKey
}

func (app *App) kvStoreKeys() map[string]*storetypes.KVStoreKey {
	keys := make(map[string]*storetypes.KVStoreKey)
	for _, k := range app.GetStoreKeys() {
		if kv, ok := k.(*storetypes.KVStoreKey); ok {
			keys[kv.Name()] = kv
		}
	}

	return keys
}

// SimulationManager implements the SimulationApp interface. We don't use simulations.
func (app *App) SimulationManager() *module.SimulationManager {
	return nil
}

// RegisterAPIRoutes registers all application module routes with the provided
// API server.
func (app *App) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	app.App.RegisterAPIRoutes(apiSvr, apiConfig)
	// register swagger API in app.go so that other applications can override easily
	if err := server.RegisterSwaggerAPI(
		apiSvr.ClientCtx, apiSvr.Router, apiConfig.Swagger,
	); err != nil {
		panic(err)
	}

	if err := app.Polaris.SetupServices(apiSvr.ClientCtx); err != nil {
		panic(err)
	}
}

// PolarisConfigFn returns a function that provides the initialization of the standard
// set of precompiles.
func PolarisConfigFn(cfg *evmconfig.Config) func() *evmconfig.Config {
	return func() *evmconfig.Config {
		return cfg
	}
}

// QueryContextFn returns a context for query requests.
func QueryContextFn(app *App) func() func(height int64, prove bool) (sdk.Context, error) {
	return func() func(height int64, prove bool) (sdk.Context, error) {
		return app.BaseApp.CreateQueryContext
	}
}

// Close shuts down the application.
func (app *App) Close() error {
	if pl := app.Polaris; pl != nil {
		return pl.Close()
	}
	return app.BaseApp.Close()
}
