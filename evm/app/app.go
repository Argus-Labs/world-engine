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
	"fmt"
	"io"
	"os"
	"path/filepath"

	"cosmossdk.io/depinject"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	evidencekeeper "cosmossdk.io/x/evidence/keeper"
	upgradekeeper "cosmossdk.io/x/upgrade/keeper"
	evmv1alpha1 "github.com/berachain/polaris/cosmos/api/polaris/evm/v1alpha1"
	evmconfig "github.com/berachain/polaris/cosmos/config"
	ethcryptocodec "github.com/berachain/polaris/cosmos/crypto/codec"
	signinglib "github.com/berachain/polaris/cosmos/lib/signing"
	polarruntime "github.com/berachain/polaris/cosmos/runtime"
	"github.com/berachain/polaris/cosmos/runtime/ante"
	"github.com/berachain/polaris/cosmos/runtime/miner"
	evmkeeper "github.com/berachain/polaris/cosmos/x/evm/keeper"
	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/types/module"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	consensuskeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	crisiskeeper "github.com/cosmos/cosmos-sdk/x/crisis/keeper"
	distrkeeper "github.com/cosmos/cosmos-sdk/x/distribution/keeper"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	mintkeeper "github.com/cosmos/cosmos-sdk/x/mint/keeper"
	slashingkeeper "github.com/cosmos/cosmos-sdk/x/slashing/keeper"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/evm/router"
	"pkg.world.dev/world-engine/evm/sequencer"
	namespacekeeper "pkg.world.dev/world-engine/evm/x/namespace/keeper"
	namespacetypes "pkg.world.dev/world-engine/evm/x/namespace/types"
	shardkeeper "pkg.world.dev/world-engine/evm/x/shard/keeper"
	shardtypes "pkg.world.dev/world-engine/evm/x/shard/types"
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
	*runtime.App
	*polarruntime.Polaris
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

	// polaris required keepers
	EVMKeeper *evmkeeper.Keeper

	// world engine keepers
	NamespaceKeeper *namespacekeeper.Keeper
	ShardKeeper     *shardkeeper.Keeper

	// plugins
	Router         router.Router
	ShardSequencer *sequencer.Sequencer

	// Flushed message cache
	shardTxDataMsgs   []*shardtypes.SubmitShardTxRequest
	shardRegisterMsgs []*namespacetypes.UpdateNamespaceRequest
}

//nolint:gochecknoinits // from sdk.
func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	DefaultNodeHome = filepath.Join(userHomeDir, ".world")
}

// NewApp returns a reference to an initialized App.
//
//nolint:funlen // its fine
func NewApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	loadLatest bool,
	bech32Prefix string,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *App {
	var (
		app        = &App{}
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
		&app.NamespaceKeeper, // Added for World Engine
		&app.ShardKeeper,     // Added for World Engine
	); err != nil {
		panic(err)
	}

	polarConfig := evmconfig.MustReadConfigFromAppOpts(appOpts)

	// Build the app using the app builder.
	app.App = appBuilder.Build(db, traceStore, baseAppOptions...)
	app.setPlugins(logger)

	app.Polaris = polarruntime.New(app,
		polarConfig, app.Logger(), app.EVMKeeper.Host, nil,
	)

	// Build cosmos ante handler for non-evm transactions.
	cosmHandler, err := authante.NewAnteHandler(
		authante.HandlerOptions{
			AccountKeeper:   app.AccountKeeper,
			BankKeeper:      app.BankKeeper,
			FeegrantKeeper:  nil,
			SigGasConsumer:  ante.EthSecp256k1SigVerificationGasConsumer,
			SignModeHandler: app.txConfig.SignModeHandler(),
			TxFeeChecker: func(_ sdk.Context, _ sdk.Tx) (sdk.Coins, int64, error) {
				return nil, 0, nil
			},
		},
	)
	if err != nil {
		panic(err)
	}

	// Setup Polaris Runtime.
	if err := app.Polaris.Build(
		app,
		cosmHandler,
		app.EVMKeeper,
		miner.DefaultAllowedMsgs,
		app.Router.PostBlockHook,
		app.PrepareProposalHook,
	); err != nil {
		panic(err)
	}

	// register streaming services
	if err := app.RegisterStreamingServices(appOpts, app.kvStoreKeys()); err != nil {
		panic(err)
	}

	/****  Module Options ****/
	app.ModuleManager.RegisterInvariants(app.CrisisKeeper)

	// RegisterUpgradeHandlers is used for registering any on-chain upgrades.
	app.RegisterUpgradeHandlers()

	// Register eth_secp256k1 keys
	ethcryptocodec.RegisterInterfaces(app.interfaceRegistry)

	// Set World Engine custom preBlocker.
	// We need this here because app.Polaris.Build is going to be injecting the app's preBlocker.
	app.SetPreBlocker(app.preBlocker)

	// Load the app
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

// preBlocker is an ABCI preBlocker hook called at the beginning of each block.
// We use this hook to execute the messages received from the World Engine shard router.
func (app *App) preBlocker(ctx sdk.Context, _ *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
	app.Logger().With("module", "app").Info("Entering EVM Base Shard preblocker")

	// Handle shard registration messages
	for _, shardRegisterMsg := range app.shardRegisterMsgs {
		app.Logger().With("module", "app").Info(
			fmt.Sprintf("Registering new shard with namespace %q to %q",
				shardRegisterMsg.Namespace.ShardName,
				shardRegisterMsg.Namespace.ShardAddress,
			),
		)
		handler := app.MsgServiceRouter().Handler(shardRegisterMsg)
		_, err := handler(ctx, shardRegisterMsg)
		// If the error is just an expected unathorize error (i.e. from an invalid namespace, etc), dont return
		// an error. Otherwise, it will cause FinalizeBlock to fail and the program will panic.
		if err != nil && !eris.Is(err, sdkerrors.ErrUnauthorized) && !eris.Is(err, sdkerrors.ErrInvalidRequest) {
			app.Logger().Error(
				fmt.Sprintf(
					"failed to register new shard with namespace %q: %q",
					shardRegisterMsg.Namespace.ShardName, err,
				),
			)

			return nil, eris.Wrapf(
				err, "failed to register new shard with namespace %q", shardRegisterMsg.Namespace.ShardName,
			)
		}
	}

	// Handle game shard transaction sequencing
	for _, shardTxDataMsg := range app.shardTxDataMsgs {
		app.Logger().With("module", "app").Info(
			fmt.Sprintf("Submitting game shard tx data to %q: %q", shardTxDataMsg.Namespace, shardTxDataMsg),
		)
		handler := app.MsgServiceRouter().Handler(shardTxDataMsg)
		_, err := handler(ctx, shardTxDataMsg)
		if err != nil && !eris.Is(err, sdkerrors.ErrUnauthorized) && !eris.Is(err, sdkerrors.ErrInvalidRequest) {
			app.Logger().Error(fmt.Sprintf("failed to process game shard tx data submission: %q", err))
			return nil, eris.Wrapf(err, "failed to process game shard tx data submission")
		}
	}

	app.Logger().With("module", "app").Info("Exiting EVM Base Shard preblocker")
	return &sdk.ResponsePreBlock{ConsensusParamsChanged: true}, nil
}

// PrepareProposalHook injects the shard router side-channel messages into ResponsePrepareProposal
// after Polaris' PrepareProposal logic is executed.
func (app *App) PrepareProposalHook(
	_ sdk.Context, _ *abci.RequestPrepareProposal, resp *abci.ResponsePrepareProposal,
) (*abci.ResponsePrepareProposal, error) {
	shardTxDataMsgs, shardRegisterMsgs := app.ShardSequencer.FlushMessages()

	// Append the game shard transaction data sequencing messages
	for _, shardTxDataMsg := range shardTxDataMsgs {
		app.Logger().Info(shardTxDataMsg.String())
		bz, err := shardTxDataMsg.Marshal()
		if err != nil {
			return nil, eris.Wrap(err, "failed to marshal game shard sequencing messages")
		}
		resp.Txs = append(resp.Txs, bz)
	}

	// Append the game shard namespace registration messages
	for _, shardRegisterMsg := range shardRegisterMsgs {
		app.Logger().Info(shardRegisterMsg.String())
		bz, err := shardRegisterMsg.Marshal()
		if err != nil {
			return nil, eris.Wrap(err, "failed to marshal game shard namespace registration messages")
		}
		resp.Txs = append(resp.Txs, bz)
	}

	app.shardTxDataMsgs = shardTxDataMsgs
	app.shardRegisterMsgs = shardRegisterMsgs

	return &abci.ResponsePrepareProposal{Txs: resp.Txs}, nil
}

// Name returns the name of the App.
func (app *App) Name() string { return app.BaseApp.Name() }

// LegacyAmino returns SimApp's amino codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *App) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
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

// Close shuts down the application.
func (app *App) Close() error {
	if pl := app.Polaris; pl != nil {
		return pl.Close()
	}
	return app.BaseApp.Close()
}
