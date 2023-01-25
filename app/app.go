package argus

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/grpc/tmservice"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/server/api"
	"github.com/cosmos/cosmos-sdk/server/config"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/simapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/cosmos/cosmos-sdk/version"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	"github.com/cosmos/cosmos-sdk/x/crisis"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	upgradetypes "github.com/cosmos/cosmos-sdk/x/upgrade/types"
	ibcclienttypes "github.com/cosmos/ibc-go/v5/modules/core/02-client/types"
	ibcchanneltypes "github.com/cosmos/ibc-go/v5/modules/core/04-channel/types"
	"github.com/evmos/ethermint/server/flags"
	etherminttypes "github.com/evmos/ethermint/types"
	"github.com/gorilla/mux"
	"github.com/rakyll/statik/fs"
	"github.com/spf13/cast"
	abci "github.com/tendermint/tendermint/abci/types"
	tmjson "github.com/tendermint/tendermint/libs/json"
	"github.com/tendermint/tendermint/libs/log"
	tmos "github.com/tendermint/tendermint/libs/os"
	dbm "github.com/tendermint/tm-db"

	argusappparams "github.com/argus-labs/argus/app/simparams"
	adaptertypes "github.com/argus-labs/argus/x/adapter"

	"github.com/argus-labs/argus/ante"
	"github.com/argus-labs/argus/app/keepers"
	"github.com/argus-labs/argus/app/upgrades"
	"github.com/argus-labs/argus/pool"
	"github.com/argus-labs/argus/sidecar"

	// unnamed import of statik for swagger UI support
	_ "github.com/cosmos/cosmos-sdk/client/docs/statik"

	// Force-load the tracer engines to trigger registration due to Go-Ethereum v1.10.15 changes
	_ "github.com/ethereum/go-ethereum/eth/tracers/js"
	_ "github.com/ethereum/go-ethereum/eth/tracers/native"
)

var (
	// DefaultNodeHome default home directories for the application daemon
	DefaultNodeHome string

	Upgrades []upgrades.Upgrade
)

var (
	_ simapp.App              = (*ArgusApp)(nil)
	_ servertypes.Application = (*ArgusApp)(nil)
)

// ArgusApp extends an ABCI application, but with most of its parameters exported.
// They are exported for convenience in creating helper functions, as object
// capabilities aren't needed for testing.
type ArgusApp struct { //nolint: revive
	*baseapp.BaseApp
	keepers.AppKeepers

	legacyAmino       *codec.LegacyAmino
	appCodec          codec.Codec
	interfaceRegistry types.InterfaceRegistry
	invCheckPeriod    uint

	// the module manager
	mm *module.Manager
	// simulation manager
	sm           *module.SimulationManager
	configurator module.Configurator

	// msgFunnel is a funnel that services can pass messages into for safely transitioning the state.
	// this is mostly used by Sidecar.
	msgPool *pool.MsgPool
}

func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	DefaultNodeHome = filepath.Join(userHomeDir, ".argus")
}

// NewArgusApp returns a reference to an initialized Argus.
func NewArgusApp(
	logger log.Logger,
	db dbm.DB, traceStore io.Writer,
	loadLatest bool,
	skipUpgradeHeights map[int64]bool,
	homePath string,
	invCheckPeriod uint,
	encodingConfig argusappparams.EncodingConfig,
	appOpts servertypes.AppOptions,
	baseAppOptions ...func(*baseapp.BaseApp),
) *ArgusApp {
	appCodec := encodingConfig.Codec
	legacyAmino := encodingConfig.Amino
	interfaceRegistry := encodingConfig.InterfaceRegistry

	bApp := baseapp.NewBaseApp(
		appName,
		logger,
		db,
		encodingConfig.TxConfig.TxDecoder(),
		baseAppOptions...)
	bApp.SetCommitMultiStoreTracer(traceStore)
	bApp.SetVersion(version.Version)
	bApp.SetInterfaceRegistry(interfaceRegistry)

	app := &ArgusApp{
		BaseApp:           bApp,
		legacyAmino:       legacyAmino,
		appCodec:          appCodec,
		interfaceRegistry: interfaceRegistry,
		invCheckPeriod:    invCheckPeriod,
	}
	// Setup keepers
	app.AppKeepers = keepers.NewAppKeeper(
		appCodec,
		bApp,
		legacyAmino,
		maccPerms,
		app.BlockedModuleAccountAddrs(),
		skipUpgradeHeights,
		homePath,
		invCheckPeriod,
		appOpts,
	)

	skipGenesisInvariants := cast.ToBool(appOpts.Get(crisis.FlagSkipGenesisInvariants))

	// NOTE: Any module instantiated in the module manager that is later modified
	// must be passed by reference here.
	app.mm = module.NewManager(appModules(app, encodingConfig, skipGenesisInvariants)...)

	// During begin block slashing happens after distr.BeginBlocker so that
	// there is nothing left over in the validator fee pool, so as to keep the
	// CanWithdrawInvariant invariant.
	// NOTE: staking module is required if HistoricalEntries param > 0
	// NOTE: capability module's beginblocker must come before any modules using capabilities (e.g. IBC)
	// Tell the app's module manager how to set the order of BeginBlockers, which are run at the beginning of every block.
	app.mm.SetOrderBeginBlockers(orderBeginBlockers()...)

	app.mm.SetOrderEndBlockers(orderEndBlockers()...)

	// NOTE: The genutils module must occur after staking so that pools are
	// properly initialized with tokens from genesis accounts.
	// NOTE: The genutils module must also occur after auth so that it can access the params from auth.
	// NOTE: Capability module must occur first so that it can initialize any capabilities
	// so that other modules that want to create or claim capabilities afterwards in InitChain
	// can do so safely.
	app.mm.SetOrderInitGenesis(orderInitBlockers()...)

	// Uncomment if you want to set a custom migration order here.
	// app.mm.SetOrderMigrations(custom order)

	app.mm.RegisterInvariants(&app.CrisisKeeper)
	app.mm.RegisterRoutes(app.Router(), app.QueryRouter(), encodingConfig.Amino)

	app.configurator = module.NewConfigurator(app.appCodec, app.MsgServiceRouter(), app.GRPCQueryRouter())
	app.mm.RegisterServices(app.configurator)

	// create the simulation manager and define the order of the modules for deterministic simulations
	//
	// NOTE: this is not required apps that don't use the simulator for fuzz testing
	// transactions
	app.sm = module.NewSimulationManager(simulationModules(app, encodingConfig, skipGenesisInvariants)...)

	app.sm.RegisterStoreDecoders()

	// initialize stores
	app.MountKVStores(app.GetKVStoreKey())
	app.MountTransientStores(app.GetTransientStoreKey())
	app.MountMemoryStores(app.GetMemoryStoreKey())

	ah, err := ante.NewEthermintAnteHandler(ante.EthermintHandlerOptions{
		AccountKeeper:          app.AccountKeeper,
		BankKeeper:             app.BankKeeper,
		IBCKeeper:              app.IBCKeeper,
		FeeMarketKeeper:        app.FeeMarketKeeper,
		EvmKeeper:              app.EvmKeeper,
		FeegrantKeeper:         app.FeeGrantKeeper,
		SignModeHandler:        app.GetTxConfig().SignModeHandler(),
		SigGasConsumer:         ante.EthermintSigVerificationGasConsumer,
		MaxTxGasWanted:         cast.ToUint64(appOpts.Get(flags.EVMMaxTxGasWanted)),
		ExtensionOptionChecker: etherminttypes.HasDynamicFeeExtensionOption,
		TxFeeChecker:           ante.NewDynamicFeeChecker(app.EvmKeeper),
	})
	if err != nil {
		panic(err)
	}
	app.SetAnteHandler(ah)
	app.SetInitChainer(app.InitChainer)
	app.SetBeginBlocker(app.BeginBlocker)
	app.SetEndBlocker(app.EndBlocker)

	app.setupUpgradeHandlers()
	app.setupUpgradeStoreLoaders()

	if loadLatest {
		if err := app.LoadLatestVersion(); err != nil {
			tmos.Exit(fmt.Sprintf("failed to load latest version: %s", err))
		}
	}

	app.msgPool = pool.NewMsgPool(10)

	startSideCarIfFlagSet(app.MsgServiceRouter(), app.GRPCQueryRouter(), app.BankKeeper, app.GetBaseApp().CommitMultiStore(), app.Logger(), app.msgPool)

	return app
}

func startSideCarIfFlagSet(msgRouter *baseapp.MsgServiceRouter, grpcRouter *baseapp.GRPCQueryRouter, bk bankkeeper.Keeper, cms sdk.CommitMultiStore, lg log.Logger, sender pool.MsgPoolSender) {
	sidecarFlag := os.Getenv("USE_SIDECAR")
	var useSidecar bool
	var err error
	if sidecarFlag != "" {
		useSidecar, err = strconv.ParseBool(sidecarFlag)
		if err != nil {
			panic(err)
		}
	}

	if useSidecar {
		err = sidecar.StartSidecar(msgRouter, grpcRouter, bk, cms, lg, sender, authtypes.NewModuleAddress(adaptertypes.Name).String())
		if err != nil {
			panic(fmt.Errorf("failed to start sidecar process: %w", err))
		}
	}
}

func GetDefaultBypassFeeMessages() []string {
	return []string{
		sdk.MsgTypeURL(&ibcchanneltypes.MsgRecvPacket{}),
		sdk.MsgTypeURL(&ibcchanneltypes.MsgAcknowledgement{}),
		sdk.MsgTypeURL(&ibcclienttypes.MsgUpdateClient{}),
	}
}

// Name returns the name of the App
func (app *ArgusApp) Name() string { return app.BaseApp.Name() }

// BeginBlocker application updates every begin block
func (app *ArgusApp) BeginBlocker(ctx sdk.Context, req abci.RequestBeginBlock) abci.ResponseBeginBlock {
	return app.mm.BeginBlock(ctx, req)
}

// EndBlocker application updates every end block
func (app *ArgusApp) EndBlocker(ctx sdk.Context, req abci.RequestEndBlock) abci.ResponseEndBlock {
	cachedCheckTx := ctx.IsCheckTx()
	msgs := app.msgPool.Drain()
	if len(msgs) != 0 {
		rtr := app.MsgServiceRouter()
		ctx = ctx.WithIsCheckTx(false)
		for _, msg := range msgs {
			h := rtr.Handler(msg)
			if h != nil {
				res, err := h(ctx, msg)
				if err != nil {
					app.Logger().Error("msg from MsgPool failed", "error", err.Error())
				}
				app.Logger().Info("handler returned without error", "result", res.String())
			}
		}
	}
	res := app.mm.EndBlock(ctx.WithIsCheckTx(cachedCheckTx), req)
	return res
}

// InitChainer application update at chain initialization
func (app *ArgusApp) InitChainer(ctx sdk.Context, req abci.RequestInitChain) abci.ResponseInitChain {
	var genesisState GenesisState
	if err := tmjson.Unmarshal(req.AppStateBytes, &genesisState); err != nil {
		panic(err)
	}

	app.UpgradeKeeper.SetModuleVersionMap(ctx, app.mm.GetVersionMap())

	return app.mm.InitGenesis(ctx, app.appCodec, genesisState)
}

// LoadHeight loads a particular height
func (app *ArgusApp) LoadHeight(height int64) error {
	return app.LoadVersion(height)
}

func (app *ArgusApp) ModuleAccountAddr(moduleName string) (string, error) {
	if _, ok := maccPerms[moduleName]; !ok {
		return "", fmt.Errorf("%s does not have a module address", moduleName)
	}
	addr := authtypes.NewModuleAddress(moduleName).String()
	return addr, nil
}

// ModuleAccountAddrs returns all the app's module account addresses.
func (app *ArgusApp) ModuleAccountAddrs() map[string]bool {
	modAccAddrs := make(map[string]bool)
	for acc := range maccPerms {
		modAccAddrs[authtypes.NewModuleAddress(acc).String()] = true
	}

	return modAccAddrs
}

// BlockedModuleAccountAddrs returns all the app's blocked module account
// addresses.
func (app *ArgusApp) BlockedModuleAccountAddrs() map[string]bool {
	modAccAddrs := app.ModuleAccountAddrs()

	// remove module accounts that are ALLOWED to received funds
	//
	// TODO: Blocked on updating to v0.46.x
	// delete(modAccAddrs, authtypes.NewModuleAddress(grouptypes.ModuleName).String())
	delete(modAccAddrs, authtypes.NewModuleAddress(govtypes.ModuleName).String())

	return modAccAddrs
}

// LegacyAmino returns ArgusApp's amino codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *ArgusApp) LegacyAmino() *codec.LegacyAmino {
	return app.legacyAmino
}

// AppCodec returns Argus's app codec.
//
// NOTE: This is solely to be used for testing purposes as it may be desirable
// for modules to register their own custom testing types.
func (app *ArgusApp) AppCodec() codec.Codec {
	return app.appCodec
}

// InterfaceRegistry returns Argus's InterfaceRegistry
func (app *ArgusApp) InterfaceRegistry() types.InterfaceRegistry {
	return app.interfaceRegistry
}

// SimulationManager implements the SimulationApp interface
func (app *ArgusApp) SimulationManager() *module.SimulationManager {
	return app.sm
}

// RegisterAPIRoutes registers all application module routes with the provided
// API server.
func (app *ArgusApp) RegisterAPIRoutes(apiSvr *api.Server, apiConfig config.APIConfig) {
	clientCtx := apiSvr.ClientCtx
	// Register new tx routes from grpc-gateway.
	authtx.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)
	// Register new tendermint queries routes from grpc-gateway.
	tmservice.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// Register legacy and grpc-gateway routes for all modules.
	ModuleBasics.RegisterGRPCGatewayRoutes(clientCtx, apiSvr.GRPCGatewayRouter)

	// register swagger API from root so that other applications can override easily
	if apiConfig.Swagger {
		RegisterSwaggerAPI(apiSvr.Router)
	}
}

// RegisterTxService implements the Application.RegisterTxService method.
func (app *ArgusApp) RegisterTxService(clientCtx client.Context) {
	authtx.RegisterTxService(app.BaseApp.GRPCQueryRouter(), clientCtx, app.BaseApp.Simulate, app.interfaceRegistry)
}

// RegisterTendermintService implements the Application.RegisterTendermintService method.
func (app *ArgusApp) RegisterTendermintService(clientCtx client.Context) {
	tmservice.RegisterTendermintService(
		clientCtx,
		app.BaseApp.GRPCQueryRouter(),
		app.interfaceRegistry,
		app.Query,
	)
}

// configure store loader that checks if version == upgradeHeight and applies store upgrades
func (app *ArgusApp) setupUpgradeStoreLoaders() {
	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(fmt.Sprintf("failed to read upgrade info from disk %s", err))
	}

	if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		return
	}

	for _, upgrade := range Upgrades {
		if upgradeInfo.Name == upgrade.UpgradeName {
			app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &upgrade.StoreUpgrades))
		}
	}
}

func (app *ArgusApp) setupUpgradeHandlers() {
	for _, upgrade := range Upgrades {
		app.UpgradeKeeper.SetUpgradeHandler(
			upgrade.UpgradeName,
			upgrade.CreateUpgradeHandler(
				app.mm,
				app.configurator,
				&app.AppKeepers,
			),
		)
	}
}

// RegisterSwaggerAPI registers swagger route with API Server
func RegisterSwaggerAPI(rtr *mux.Router) {
	statikFS, err := fs.New()
	if err != nil {
		panic(err)
	}

	staticServer := http.FileServer(statikFS)
	rtr.PathPrefix("/swagger/").Handler(http.StripPrefix("/swagger/", staticServer))
}

func (app *ArgusApp) OnTxSucceeded(ctx sdk.Context, sourcePort, sourceChannel string, txHash []byte, txBytes []byte) {
}

func (app *ArgusApp) OnTxFailed(ctx sdk.Context, sourcePort, sourceChannel string, txHash []byte, txBytes []byte) {
}

// TestingApp functions

// GetBaseApp implements the TestingApp interface.
func (app *ArgusApp) GetBaseApp() *baseapp.BaseApp {
	return app.BaseApp
}

// GetTxConfig implements the TestingApp interface.
func (app *ArgusApp) GetTxConfig() client.TxConfig {
	return MakeTestEncodingConfig().TxConfig
}

// EmptyAppOptions is a stub implementing AppOptions
type EmptyAppOptions struct{}

// Get implements AppOptions
func (ao EmptyAppOptions) Get(o string) interface{} {
	return nil
}
