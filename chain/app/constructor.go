package argus

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"time"

	codecTypes "github.com/cosmos/cosmos-sdk/codec/types"
	grpc2 "github.com/cosmos/cosmos-sdk/server/grpc"
	crgserver "github.com/cosmos/cosmos-sdk/server/rosetta/lib/server"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/tx"
	abciclient "github.com/tendermint/tendermint/abci/client"
	"github.com/tendermint/tendermint/node"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/codec"
	sdkServer "github.com/cosmos/cosmos-sdk/server"
	"github.com/cosmos/cosmos-sdk/server/api"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	"github.com/cosmos/cosmos-sdk/server/rosetta"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/snapshots"
	snapshottypes "github.com/cosmos/cosmos-sdk/snapshots/types"
	"github.com/cosmos/cosmos-sdk/store"
	"github.com/cosmos/cosmos-sdk/telemetry"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cast"
	"github.com/tendermint/tendermint/libs/log"
	"github.com/tendermint/tendermint/p2p"
	"github.com/tendermint/tendermint/privval"
	dbm "github.com/tendermint/tm-db"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/argus-labs/argus/app/simparams"

	tmservice "github.com/tendermint/tendermint/libs/service"

	rollconf "github.com/rollkit/rollkit/config"
	rollconv "github.com/rollkit/rollkit/conv"
	rollnode "github.com/rollkit/rollkit/node"
	rollrpc "github.com/rollkit/rollkit/rpc"
)

type AppCreator struct {
	// TODO(technicallyty): here we should add fields for hooks and stuff that can be injected in NewApp.
	EncCfg simparams.EncodingConfig
}

func (ac AppCreator) NewApp(
	logger log.Logger,
	db dbm.DB,
	traceStore io.Writer,
	appOpts servertypes.AppOptions,
) servertypes.Application {
	var cache sdk.MultiStorePersistentCache

	if cast.ToBool(appOpts.Get(sdkServer.FlagInterBlockCache)) {
		cache = store.NewCommitKVStoreCacheManager()
	}

	skipUpgradeHeights := make(map[int64]bool)
	for _, h := range cast.ToIntSlice(appOpts.Get(sdkServer.FlagUnsafeSkipUpgrades)) {
		skipUpgradeHeights[int64(h)] = true
	}

	pruningOpts, err := sdkServer.GetPruningOptionsFromFlags(appOpts)
	if err != nil {
		panic(err)
	}

	snapshotDir := filepath.Join(cast.ToString(appOpts.Get(flags.FlagHome)), "data", "snapshots")
	snapshotDB, err := dbm.NewDB("metadata", sdkServer.GetAppDBBackend(appOpts), snapshotDir)
	if err != nil {
		panic(err)
	}
	snapshotStore, err := snapshots.NewStore(snapshotDB, snapshotDir)
	if err != nil {
		panic(err)
	}
	snapshotOptions := snapshottypes.NewSnapshotOptions(
		cast.ToUint64(appOpts.Get(sdkServer.FlagStateSyncSnapshotInterval)),
		cast.ToUint32(appOpts.Get(sdkServer.FlagStateSyncSnapshotKeepRecent)),
	)

	app := NewArgusApp(
		logger, db, traceStore, true, skipUpgradeHeights,
		cast.ToString(appOpts.Get(flags.FlagHome)),
		cast.ToUint(appOpts.Get(sdkServer.FlagInvCheckPeriod)),
		ac.EncCfg,
		appOpts,
		baseapp.SetPruning(pruningOpts),
		baseapp.SetMinGasPrices(cast.ToString(appOpts.Get(sdkServer.FlagMinGasPrices))),
		baseapp.SetHaltHeight(cast.ToUint64(appOpts.Get(sdkServer.FlagHaltHeight))),
		baseapp.SetHaltTime(cast.ToUint64(appOpts.Get(sdkServer.FlagHaltTime))),
		baseapp.SetMinRetainBlocks(cast.ToUint64(appOpts.Get(sdkServer.FlagMinRetainBlocks))),
		baseapp.SetInterBlockCache(cache),
		baseapp.SetTrace(cast.ToBool(appOpts.Get(sdkServer.FlagTrace))),
		baseapp.SetIndexEvents(cast.ToStringSlice(appOpts.Get(sdkServer.FlagIndexEvents))),
		baseapp.SetSnapshot(snapshotStore, snapshotOptions),
		baseapp.SetIAVLCacheSize(cast.ToInt(appOpts.Get(sdkServer.FlagIAVLCacheSize))),
		baseapp.SetIAVLDisableFastNode(cast.ToBool(appOpts.Get(sdkServer.FlagDisableIAVLFastNode))),
	)
	// TODO(technicallyty): alter app here with the constructor struct. we want to inject hooks and stuff into the app.
	return app
}

// Flags for application
const (
	// Tendermint full-node start flags
	flagWithTendermint     = "with-tendermint"
	flagAddress            = "address"
	flagTransport          = "transport"
	flagTraceStore         = "trace-store"
	flagCPUProfile         = "cpu-profile"
	FlagMinGasPrices       = "minimum-gas-prices"
	FlagHaltHeight         = "halt-height"
	FlagHaltTime           = "halt-time"
	FlagInterBlockCache    = "inter-block-cache"
	FlagUnsafeSkipUpgrades = "unsafe-skip-upgrades"
	FlagTrace              = "trace"
	FlagInvCheckPeriod     = "inv-check-period"

	FlagPruning             = "pruning"
	FlagPruningKeepRecent   = "pruning-keep-recent"
	FlagPruningInterval     = "pruning-interval"
	FlagIndexEvents         = "index-events"
	FlagMinRetainBlocks     = "min-retain-blocks"
	FlagIAVLCacheSize       = "iavl-cache-size"
	FlagDisableIAVLFastNode = "iavl-disable-fastnode"

	// state sync-related flags
	FlagStateSyncSnapshotInterval   = "state-sync.snapshot-interval"
	FlagStateSyncSnapshotKeepRecent = "state-sync.snapshot-keep-recent"

	// api-related flags
	FlagAPIEnable             = "api.enable"
	FlagAPISwagger            = "api.swagger"
	FlagAPIAddress            = "api.address"
	FlagAPIMaxOpenConnections = "api.max-open-connections"
	FlagRPCReadTimeout        = "api.rpc-read-timeout"
	FlagRPCWriteTimeout       = "api.rpc-write-timeout"
	FlagRPCMaxBodyBytes       = "api.rpc-max-body-bytes"
	FlagAPIEnableUnsafeCORS   = "api.enabled-unsafe-cors"

	// gRPC-related flags
	flagGRPCOnly       = "grpc-only"
	flagGRPCEnable     = "grpc.enable"
	flagGRPCAddress    = "grpc.address"
	flagGRPCWebEnable  = "grpc-web.enable"
	flagGRPCWebAddress = "grpc-web.address"
)

// Start starts the rollup in process.
func Start(ctx *sdkServer.Context, clientCtx client.Context, serverCfg *serverconfig.Config, appCreator servertypes.AppCreator) error {
	cfg := ctx.Config
	home := cfg.RootDir
	var cpuProfileCleanup func()

	if cpuProfile := ctx.Viper.GetString(flagCPUProfile); cpuProfile != "" {
		f, err := os.Create(cpuProfile)
		if err != nil {
			return err
		}

		ctx.Logger.Info("starting CPU profiler", "profile", cpuProfile)
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}

		cpuProfileCleanup = func() {
			ctx.Logger.Info("stopping CPU profiler", "profile", cpuProfile)
			pprof.StopCPUProfile()
			if err := f.Close(); err != nil {
				ctx.Logger.Info("failed to close cpu-profile file", "profile", cpuProfile, "err", err.Error())
			}
		}
	}

	db, err := openDB(home, sdkServer.GetAppDBBackend(ctx.Viper))
	if err != nil {
		return err
	}

	traceWriterFile := ctx.Viper.GetString(flagTraceStore)
	traceWriter, err := openTraceWriter(traceWriterFile)
	if err != nil {
		return err
	}

	config := serverCfg

	if err := config.ValidateBasic(); err != nil {
		return err
	}

	app := appCreator(ctx.Logger, db, traceWriter, ctx.Viper)

	genDocProvider := node.DefaultGenesisDocProviderFunc(cfg)
	genDoc, err := genDocProvider()
	if err != nil {
		return err
	}

	var (
		tmNode   tmservice.Service
		server   *rollrpc.Server
		gRPCOnly = ctx.Viper.GetBool(flagGRPCOnly)
	)

	if gRPCOnly {
		ctx.Logger.Info("starting node in gRPC only mode; Tendermint is disabled")
		config.GRPC.Enable = true
	} else {
		ctx.Logger.Info("starting node with Rollkit in-process")

		nodeKey, err := p2p.LoadOrGenNodeKey(cfg.NodeKeyFile())
		if err != nil {
			return err
		}
		pval := privval.LoadOrGenFilePV(cfg.PrivValidatorKeyFile(), cfg.PrivValidatorStateFile())
		// keys in Rollkit format
		p2pKey, err := rollconv.GetNodeKey(nodeKey)
		if err != nil {
			return err
		}
		signingKey, err := rollconv.GetNodeKey(&p2p.NodeKey{PrivKey: pval.Key.PrivKey})
		if err != nil {
			return err
		}

		nodeConfig := rollconf.NodeConfig{}
		err = nodeConfig.GetViperConfig(ctx.Viper)
		if err != nil {
			return err
		}
		rollconv.GetNodeConfig(&nodeConfig, cfg)
		err = rollconv.TranslateAddresses(&nodeConfig)
		if err != nil {
			return err
		}
		nodeConfig.DALayer = "mock"
		tmNode, err := rollnode.NewNode(
			context.Background(),
			nodeConfig,
			p2pKey,
			signingKey,
			abciclient.NewLocalClient(nil, app),
			genDoc,
			ctx.Logger,
		)
		if err != nil {
			return err
		}

		server := rollrpc.NewServer(tmNode, cfg.RPC, ctx.Logger)
		err = server.Start()
		if err != nil {
			return err
		}

		if err := tmNode.Start(); err != nil {
			return err
		}
	}

	// Add the tx service to the gRPC router. We only need to register this
	// service if API or gRPC is enabled, and avoid doing so in the general
	// case, because it spawns a new local tendermint RPC client.
	if (config.API.Enable || config.GRPC.Enable) && tmNode != nil {
		// re-assign for making the client available below
		// do not use := to avoid shadowing clientCtx
		clientCtx = clientCtx.WithClient(server.Client())

		app.RegisterTxService(clientCtx)
		app.RegisterTendermintService(clientCtx)

		if a, ok := app.(servertypes.ApplicationQueryService); ok {
			a.RegisterNodeService(clientCtx)
		}
	}

	metrics, err := startTelemetry(*config)
	if err != nil {
		return err
	}

	var apiSrv *api.Server
	if config.API.Enable {

		clientCtx := clientCtx.WithHomeDir(home).WithChainID(genDoc.ChainID)

		if config.GRPC.Enable {
			_, port, err := net.SplitHostPort(config.GRPC.Address)
			if err != nil {
				return err
			}

			maxSendMsgSize := config.GRPC.MaxSendMsgSize
			if maxSendMsgSize == 0 {
				maxSendMsgSize = serverconfig.DefaultGRPCMaxSendMsgSize
			}

			maxRecvMsgSize := config.GRPC.MaxRecvMsgSize
			if maxRecvMsgSize == 0 {
				maxRecvMsgSize = serverconfig.DefaultGRPCMaxRecvMsgSize
			}

			grpcAddress := fmt.Sprintf("127.0.0.1:%s", port)

			// If grpc is enabled, configure grpc client for grpc gateway.
			grpcClient, err := grpc.Dial(
				grpcAddress,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithDefaultCallOptions(
					grpc.ForceCodec(codec.NewProtoCodec(clientCtx.InterfaceRegistry).GRPCCodec()),
					grpc.MaxCallRecvMsgSize(maxRecvMsgSize),
					grpc.MaxCallSendMsgSize(maxSendMsgSize),
				),
			)
			if err != nil {
				return err
			}

			clientCtx = clientCtx.WithGRPCClient(grpcClient)
			ctx.Logger.Debug("grpc client assigned to client context", "target", grpcAddress)
		}

		apiSrv = api.New(clientCtx, ctx.Logger.With("module", "api-server"))
		app.RegisterAPIRoutes(apiSrv, config.API)
		if config.Telemetry.Enabled {
			apiSrv.SetTelemetry(metrics)
		}
		errCh := make(chan error)

		go func() {
			if err := apiSrv.Start(*config); err != nil {
				errCh <- err
			}
		}()

		select {
		case err := <-errCh:
			return err

		case <-time.After(servertypes.ServerStartTime): // assume server started successfully
		}
	}

	var (
		grpcSrv    *grpc.Server
		grpcWebSrv *http.Server
	)

	argusApp := app.(*ArgusApp)
	pCdc := protoCodec{
		Codec: argusApp.appCodec,
		ir:    argusApp.interfaceRegistry,
	}
	clientCtx.TxConfig = tx.NewTxConfig(pCdc, []signing.SignMode{signing.SignMode_SIGN_MODE_DIRECT})
	clientCtx.InterfaceRegistry = argusApp.InterfaceRegistry()

	if config.GRPC.Enable {
		grpcSrv, err = grpc2.StartGRPCServer(clientCtx, app, config.GRPC)
		if err != nil {
			return err
		}
		defer grpcSrv.Stop()
		if config.GRPCWeb.Enable {
			grpcWebSrv, err = grpc2.StartGRPCWeb(grpcSrv, *config)
			if err != nil {
				ctx.Logger.Error("failed to start grpc-web http server: ", err)
				return err
			}
			defer func() {
				if err := grpcWebSrv.Close(); err != nil {
					ctx.Logger.Error("failed to close grpc-web http server: ", err)
				}
			}()
		}
	}

	// At this point it is safe to block the process if we're in gRPC only mode as
	// we do not need to start Rosetta or handle any Tendermint related processes.
	if gRPCOnly {
		// wait for signal capture and gracefully return
		return sdkServer.WaitForQuitSignals()
	}

	var rosettaSrv crgserver.Server
	if config.Rosetta.Enable {
		offlineMode := config.Rosetta.Offline

		// If GRPC is not enabled rosetta cannot work in online mode, so it works in
		// offline mode.
		if !config.GRPC.Enable {
			offlineMode = true
		}

		minGasPrices, err := sdk.ParseDecCoins(config.MinGasPrices)
		if err != nil {
			ctx.Logger.Error("failed to parse minimum-gas-prices: ", err)
			return err
		}

		conf := &rosetta.Config{
			Blockchain:          config.Rosetta.Blockchain,
			Network:             config.Rosetta.Network,
			TendermintRPC:       ctx.Config.RPC.ListenAddress,
			GRPCEndpoint:        config.GRPC.Address,
			Addr:                config.Rosetta.Address,
			Retries:             config.Rosetta.Retries,
			Offline:             offlineMode,
			GasToSuggest:        config.Rosetta.GasToSuggest,
			EnableFeeSuggestion: config.Rosetta.EnableFeeSuggestion,
			GasPrices:           minGasPrices.Sort(),
			Codec:               clientCtx.Codec.(*codec.ProtoCodec),
			InterfaceRegistry:   clientCtx.InterfaceRegistry,
		}

		rosettaSrv, err = rosetta.ServerFromConfig(conf)
		if err != nil {
			return err
		}

		errCh := make(chan error)
		go func() {
			if err := rosettaSrv.Start(); err != nil {
				errCh <- err
			}
		}()

		select {
		case err := <-errCh:
			return err

		case <-time.After(servertypes.ServerStartTime): // assume server started successfully
		}
	}

	defer func() {
		if tmNode != nil && tmNode.IsRunning() {
			_ = tmNode.Stop()
		}

		if cpuProfileCleanup != nil {
			cpuProfileCleanup()
		}

		if apiSrv != nil {
			_ = apiSrv.Close()
		}

		ctx.Logger.Info("exiting...")
	}()

	// wait for signal capture and gracefully return
	return sdkServer.WaitForQuitSignals()
}

type protoCodec struct {
	codec.Codec
	ir codecTypes.InterfaceRegistry
}

func (p protoCodec) InterfaceRegistry() codecTypes.InterfaceRegistry {
	return p.ir
}

func openDB(rootDir string, backendType dbm.BackendType) (dbm.DB, error) {
	dataDir := filepath.Join(rootDir, "data")
	return dbm.NewDB("application", backendType, dataDir)
}

func openTraceWriter(traceWriterFile string) (w io.Writer, err error) {
	if traceWriterFile == "" {
		return
	}
	return os.OpenFile(
		traceWriterFile,
		os.O_WRONLY|os.O_APPEND|os.O_CREATE,
		0o666,
	)
}

func startTelemetry(cfg serverconfig.Config) (*telemetry.Metrics, error) {
	if !cfg.Telemetry.Enabled {
		return nil, nil
	}
	return telemetry.New(cfg.Telemetry)
}
