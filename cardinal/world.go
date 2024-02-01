package cardinal

import (
	"context"
	"errors"
	"fmt"
	"github.com/goccy/go-json"
	"github.com/rs/zerolog"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"pkg.world.dev/world-engine/cardinal/router"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/gamestage"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/statsd"
)

// WorldStateType is the current state of the engine.
type WorldStateType string

const (
	DefaultHistoricalTicksToStore = 10
)

const (
	WorldStateInit       WorldStateType = "WorldStateInit"
	WorldStateRecovering WorldStateType = "WordlStateRecovering"
	WorldStateReady      WorldStateType = "WorldStateReady"
	WorldStateRunning    WorldStateType = "WorldStateRunning"
)

var (
	ErrEntitiesCreatedBeforeStartGame     = errors.New("entities should not be created before start game")
	ErrEntitiesCreatedBeforeLoadGameState = errors.New("entities should not be created before loading game state")
)

type World struct {
	engine          *ecs.Engine
	server          *server.Server
	tickChannel     <-chan time.Time
	tickDoneChannel chan<- uint64
	serverOptions   []server.Option
	cleanup         func()
	Logger          *zerolog.Logger

	// gameSequenceStage describes what stage the game is in (e.g. starting, running, shut down, etc)
	gameSequenceStage gamestage.Atomic
	endStartGame      chan bool

	// Scott's new stuff
	WorldState    WorldStateType
	msgManager    *message.Manager
	systemManager *system.Manager

	// Imported from Engine
	namespace              Namespace
	redisStorage           *redis.Storage
	entityStore            gamestate.Manager
	tick                   *atomic.Uint64
	timestamp              *atomic.Uint64
	nameToComponent        map[string]types.ComponentMetadata
	nameToQuery            map[string]engine.Query
	registeredComponents   []types.ComponentMetadata
	registeredQueries      []engine.Query
	isComponentsRegistered bool

	evmTxReceipts map[string]EVMTxReceipt

	txQueue *txpool.TxQueue

	receiptHistory *receipt.History

	chain adapter.Adapter
	// isRecovering indicates that the engine is recovering from the DA layer.
	// this is used to prevent ticks from submitting duplicate transactions the DA layer.
	isRecovering atomic.Bool

	endGameLoopCh     chan bool
	isGameLoopRunning atomic.Bool

	nextComponentID types.ComponentID

	eventHub *events.EventHub

	// addChannelWaitingForNextTick accepts a channel which will be closed after a tick has been completed.
	addChannelWaitingForNextTick chan chan struct{}

	shutdownMutex sync.Mutex
}

// NewWorld creates a new World object using Redis as the storage layer.
func NewWorld(opts ...WorldOption) (*World, error) {
	serverOptions, cardinalOptions := separateOptions(opts)

	// Load config. Fallback value is used if it's not set.
	cfg := getWorldConfig()
	if err := cfg.Validate(); err != nil {
		return nil, eris.Wrapf(err, "invalid configuration")
	}

	if err := setLogLevel(cfg.CardinalLogLevel); err != nil {
		return nil, eris.Wrap(err, "")
	}

	log.Logger.Info().Msgf("Starting a new Cardinal world in %s mode", cfg.CardinalMode)
	if cfg.CardinalMode == RunModeDev {
		ecsOptions = append(ecsOptions, ecs.WithPrettyLog())
		serverOptions = append(serverOptions, server.WithPrettyPrint())
	}
	redisStore := redis.NewRedisStorage(redis.Options{
		Addr:     cfg.RedisAddress,
		Password: cfg.RedisPassword,
		DB:       0, // use default DB
	}, cfg.CardinalNamespace)
	entityCommandBuffer, err := gamestate.NewEntityCommandBuffer(redisStore.Client)
	if err != nil {
		return nil, err
	}

	if cfg.CardinalMode == RunModeProd {
		rtr, err := router.New(cfg.BaseShardSequencerAddress, cfg.BaseShardQueryAddress, eng)
		if err != nil {
			return nil, err
		}
		eng.SetRouter(rtr)
	}

	var metricTags []string
	if cfg.CardinalMode != "" {
		metricTags = append(metricTags, string("cardinal_mode:"+cfg.CardinalMode))
	}
	if cfg.CardinalNamespace != "" {
		metricTags = append(metricTags, "cardinal_namespace:"+cfg.CardinalNamespace)
	}

	if cfg.StatsdAddress != "" || cfg.TraceAddress != "" {
		if err = statsd.Init(cfg.StatsdAddress, cfg.TraceAddress, metricTags); err != nil {
			return nil, eris.Wrap(err, "unable to init statsd")
		}
	} else {
		log.Logger.Warn().Msg("statsd is disabled")
	}

	world := &World{
		serverOptions:     serverOptions,
		endStartGame:      make(chan bool),
		gameSequenceStage: gamestage.NewAtomic(),

		// Scott's new stuff
		WorldState:    WorldStateInit,
		msgManager:    message.NewManager(),
		systemManager: system.NewManager(),

		// Imported from engine
		redisStorage:      &redisStore,
		entityStore:       entityCommandBuffer,
		namespace:         Namespace(cfg.CardinalNamespace),
		tick:              &atomic.Uint64{},
		timestamp:         new(atomic.Uint64),
		nameToComponent:   make(map[string]types.ComponentMetadata),
		nameToQuery:       make(map[string]engine.Query),
		txQueue:           txpool.NewTxQueue(),
		Logger:            &log.Logger,
		isGameLoopRunning: atomic.Bool{},
		endGameLoopCh:     make(chan bool),
		nextComponentID:   1,
		evmTxReceipts:     make(map[string]EVMTxReceipt),

		addChannelWaitingForNextTick: make(chan chan struct{}),
	}
	world.isGameLoopRunning.Store(false)
	world.registerInternalPlugin()

	// Apply options
	for _, opt := range cardinalOptions {
		opt(world)
	}

	if world.receiptHistory == nil {
		world.receiptHistory = receipt.NewHistory(world.CurrentTick(), DefaultHistoricalTicksToStore)
	}
	if world.eventHub == nil {
		world.eventHub = events.NewEventHub()
	}

	return world, nil
}

// NewMockWorld creates a World object that uses miniredis as the storage layer suitable for local development.
// If you are creating a World for unit tests, use NewTestWorld.
func NewMockWorld(opts ...WorldOption) (*World, error) {
	world, err := NewWorld(append(opts, withMockRedis())...)
	if err != nil {
		return world, err
	}
	return world, nil
}

func (w *World) CurrentTick() uint64 {
	return w.tick.Load()
}

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (w *World) Tick(ctx context.Context) error {
	// If the engine is not ready, we don't want to tick the engine.
	// Instead, we want to make sure we have recovered the state of the engine.
	if w.WorldState != WorldStateReady && w.WorldState != WorldStateRunning {
		return eris.New("must load state before first tick")
	}
	w.WorldState = WorldStateRunning

	// This defer is here to catch any panics that occur during the tick. It will log the current tick and the
	// current system that is running.
	defer func() {
		if panicValue := recover(); panicValue != nil {
			w.Logger.Error().Msgf("Tick: %d, Current running system: %s", w.CurrentTick(),
				w.systemManager.GetCurrentSystem())
			panic(panicValue)
		}
	}()

	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "cardinal.span.tick")
	defer func() {
		span.Finish()
	}()

	w.Logger.Info().Int("tick", int(w.CurrentTick())).Msg("Tick started")

	// Copy the transactions from the queue so that we can safely modify the queue while the tick is running.
	txQueue := w.txQueue.CopyTransactions()

	if err := w.entityStore.StartNextTick(w.msgManager.GetRegisteredMessages(), txQueue); err != nil {
		return err
	}

	// Set the timestamp for this tick
	startTime := time.Now()
	w.timestamp.Store(uint64(startTime.Unix()))

	// Create the engine context to inject into systems
	eCtx := NewWorldContextForTick(w, txQueue, w.Logger)

	// Run all registered systems.
	// This will run the registsred init systems if the current tick is 0
	if err := w.systemManager.RunSystems(eCtx); err != nil {
		return err
	}

	if w.eventHub != nil {
		// engine can be optionally loaded with or without an eventHub. If there is one, on every tick it must flush events.
		flushEventStart := time.Now()
		w.eventHub.FlushEvents()
		statsd.EmitTickStat(flushEventStart, "flush_events")
	}

	finalizeTickStartTime := time.Now()
	if err := w.entityStore.FinalizeTick(ctx); err != nil {
		return err
	}
	statsd.EmitTickStat(finalizeTickStartTime, "finalize")

	w.setEvmResults(txQueue.GetEVMTxs())
	if txQueue.GetAmountOfTxs() != 0 && w.chain != nil && !w.isRecovering.Load() {
		err := w.chain.Submit(ctx, txQueue.Transactions(), w.namespace.String(), w.tick.Load(), w.timestamp.Load())
		if err != nil {
			return fmt.Errorf("failed to submit transactions to base shard: %w", err)
		}
	}

	w.tick.Add(1)
	w.receiptHistory.NextTick()
	statsd.EmitTickStat(startTime, "full_tick")
	if err := statsd.Client().Count("num_of_txs", int64(txQueue.GetAmountOfTxs()), nil, 1); err != nil {
		w.Logger.Warn().Msgf("failed to emit count stat:%v", err)
	}

	return nil
}

// StartGame starts running the world game loop. Each time a message arrives on the tickChannel, a world tick is
// attempted. In addition, an HTTP server (listening on the given port) is created so that game messages can be sent
// to this world. After StartGame is called, RegisterComponent, RegisterMessages, RegisterQueries, and RegisterSystems
// may not be called. If StartGame doesn't encounter any errors, it will block forever, running the server and ticking
// the game in the background.
func (w *World) StartGame() error {
	ok := w.gameSequenceStage.CompareAndSwap(gamestage.StagePreStart, gamestage.StageStarting)
	if !ok {
		return errors.New("game has already been started")
	}

	if err := w.LoadGameState(); err != nil {
		if errors.Is(err, ErrEntitiesCreatedBeforeLoadGameState) {
			return eris.Wrap(ErrEntitiesCreatedBeforeStartGame, "")
		}
		return err
	}

	var err error
	w.server, err = server.New(w.instance, w.instance.GetEventHub().NewWebSocketEventHandler(), w.serverOptions...)
	if err != nil {
		return err
	}

	if err := w.instance.RunRouter(); err != nil {
		return eris.Wrap(err, "failed to start router service")
	}

	if w.tickChannel == nil {
		w.tickChannel = time.Tick(time.Second) //nolint:staticcheck // its ok.
	}

	w.startGameLoop(context.Background(), w.tickChannel, w.tickDoneChannel)

	go func() {
		ok := w.gameSequenceStage.CompareAndSwap(gamestage.StageStarting, gamestage.StageRunning)
		if !ok {
			log.Fatal().Msg("game was started prematurely")
		}
		if err := w.server.Serve(); errors.Is(err, http.ErrServerClosed) {
			log.Info().Err(err).Msgf("the server has been closed: %s", eris.ToString(err, true))
		} else if err != nil {
			log.Fatal().Err(err).Msgf("the server has failed: %s", eris.ToString(err, true))
		}
	}()

	// handle shutdown via a signal
	w.handleShutdown()
	<-w.endStartGame
	return err
}

func (w *World) startGameLoop(ctx context.Context, tickStart <-chan time.Time, tickDone chan<- uint64) {
	w.Logger.Info().Msg("Game loop started")
	ecslog.World(w.Logger, w, zerolog.InfoLevel)
	w.emitResourcesWarnings()

	go func() {
		ok := w.isGameLoopRunning.CompareAndSwap(false, true)
		if !ok {
			// The game has already started
			return
		}
		var waitingChs []chan struct{}
	loop:
		for {
			select {
			case <-tickStart:
				w.tickTheEngine(ctx, tickDone)
				closeAllChannels(waitingChs)
				waitingChs = waitingChs[:0]
			case <-w.endGameLoopCh:
				w.drainChannelsWaitingForNextTick()
				w.drainEndLoopChannels()
				closeAllChannels(waitingChs)
				if w.txQueue.GetAmountOfTxs() > 0 {
					// immediately tick if queue is not empty to process all txs if queue is not empty.
					w.tickTheEngine(ctx, tickDone)
					if tickDone != nil {
						close(tickDone)
					}
				}
				break loop
			case ch := <-w.addChannelWaitingForNextTick:
				waitingChs = append(waitingChs, ch)
			}
		}
		w.isGameLoopRunning.Store(false)
	}()
}

func (w *World) tickTheEngine(ctx context.Context, tickDone chan<- uint64) {
	currTick := w.CurrentTick()
	// this is the final point where errors bubble up and hit a panic. There are other places where this occurs
	// but this is the highest terminal point.
	// the panic may point you to here, (or the tick function) but the real stack trace is in the error message.
	if err := w.Tick(ctx); err != nil {
		bytes, err := json.Marshal(eris.ToJSON(err, true))
		if err != nil {
			panic(err)
		}
		w.Logger.Panic().Err(err).Str("tickError", "Error running Tick in Game Loop.").RawJSON("error", bytes)
	}
	if tickDone != nil {
		tickDone <- currTick
	}
}

func (w *World) emitResourcesWarnings() {
	// todo: add links to docs related to each warning
	if !w.isComponentsRegistered {
		w.Logger.Warn().Msg("No components registered.")
	}
	if !w.msgManager.IsMessagesRegistered() {
		w.Logger.Warn().Msg("No messages registered.")
	}
	if len(w.registeredQueries) == 0 {
		w.Logger.Warn().Msg("No queries registered.")
	}
	if !w.systemManager.IsSystemsRegistered() {
		w.Logger.Warn().Msg("No systems registered.")
	}
}

func (w *World) IsGameRunning() bool {
	return w.gameSequenceStage.Load() == gamestage.StageRunning
}

func (w *World) Shutdown() error {
	if w.cleanup != nil {
		w.cleanup()
	}
	// ok := w.gameSequenceStage.CompareAndSwap(gamestage.StageRunning, gamestage.StageShuttingDown)
	// if !ok {
	//	// Either the world hasn't been started, or we've already shut down.
	//	return nil
	// }
	// The CompareAndSwap returned true, so this call is responsible for actually
	// shutting down the game.
	defer func() {
		w.gameSequenceStage.Store(gamestage.StageShutDown)
	}()
	if w.server != nil {
		if err := w.server.Shutdown(); err != nil {
			return err
		}
	}

	w.shutdownMutex.Lock() // This queues up Shutdown calls so they happen one after the other.
	defer w.shutdownMutex.Unlock()
	if !w.isGameLoopRunning.Load() {
		return nil
	}

	log.Info().Msg("Shutting down game loop.")
	w.endGameLoopCh <- true
	for w.isGameLoopRunning.Load() { // Block until loop stops.
		time.Sleep(100 * time.Millisecond) //nolint:gomnd // its ok.
	}
	log.Info().Msg("Successfully shut down game loop.")
	if w.eventHub != nil {
		w.eventHub.Shutdown()
	}
	log.Info().Msg("Closing storage connection.")
	err := w.redisStorage.Close()
	if err != nil {
		log.Error().Err(err).Msg("Failed to close storage connection.")
		return err
	}
	log.Info().Msg("Successfully closed storage connection.")

	return nil
}

func (w *World) ListQueries() []engine.Query   { return w.registeredQueries }
func (w *World) ListMessages() []types.Message { return w.msgManager.GetRegisteredMessages() }

// logAndPanic logs the given error and panics. An error is returned so the syntax:
// return logAndPanic(eCtx, err)
// can be used at the end of state-mutating methods. This method will never actually return.
func logAndPanic(eCtx engine.Context, err error) error {
	// If the context is read-only, we don't want to panic. We just want to log the error and return it.
	if eCtx.IsReadOnly() {
		return err
	}
	eCtx.Logger().Panic().Err(err).Msgf("fatal error: %v", eris.ToString(err, true))
	return err
}

func setLogLevel(levelStr string) error {
	if levelStr == "" {
		return eris.New("log level must not be empty")
	}
	level, err := zerolog.ParseLevel(levelStr)
	if err != nil {
		var exampleLogLevels = strings.Join([]string{
			zerolog.DebugLevel.String(),
			zerolog.InfoLevel.String(),
			zerolog.WarnLevel.String(),
			zerolog.ErrorLevel.String(),
			zerolog.Disabled.String(),
		}, ", ")
		return eris.Errorf("log level %q is invalid, try one of: %v.", levelStr, exampleLogLevels)
	}
	zerolog.SetGlobalLevel(level)
	return nil
}

func applyProductionOptions(
	cfg WorldConfig,
	cardinalOptions *[]Option,
) error {
	log.Logger.Info().Msg("Starting a new Cardinal world in production mode")
	if cfg.RedisPassword == "" {
		return eris.New("REDIS_PASSWORD is required in production")
	}
	if cfg.CardinalNamespace == DefaultNamespace {
		return eris.New(
			"CARDINAL_NAMESPACE cannot be the default value in production to avoid replay attack",
		)
	}
	if cfg.BaseShardSequencerAddress == "" || cfg.BaseShardQueryAddress == "" {
		return eris.New("must supply BASE_SHARD_SEQUENCER_ADDRESS and BASE_SHARD_QUERY_ADDRESS for production " +
			"mode Cardinal worlds")
	}
	adpt, err := adapter.New(adapter.Config{
		ShardSequencerAddr: cfg.BaseShardSequencerAddress,
		EVMBaseShardAddr:   cfg.BaseShardQueryAddress,
	})
	if err != nil {
		return eris.Wrapf(err, "failed to instantiate adapter")
	}
	*cardinalOptions = append(*cardinalOptions, WithAdapter(adpt).cardinalOption)
	return nil
}

func (w *World) handleShutdown() {
	signalChannel := make(chan os.Signal, 1)
	go func() {
		signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)
		for sig := range signalChannel {
			if sig == syscall.SIGINT || sig == syscall.SIGTERM {
				err := w.Shutdown()
				if err != nil {
					log.Err(err).Msgf("There was an error during shutdown.")
				}
				return
			}
		}
	}()
}

func (w *World) RegisterPlugin(plugin Plugin) {
	if err := plugin.Register(w); err != nil {
		log.Fatal().Err(err).Msgf("failed to register plugin: %v", err)
	}
}

func (w *World) registerInternalPlugin() {
	// Register Persona plugin
	w.RegisterPlugin(newPersonaPlugin())

	// Register Debug plugin
	w.RegisterPlugin(newDebugPlugin())

	// Register CQL plugin
	w.RegisterPlugin(newCQLPlugin())

	// Register Receipt plugin
	w.RegisterPlugin(newReceiptPlugin())
}

func closeAllChannels(chs []chan struct{}) {
	for _, ch := range chs {
		close(ch)
	}
}

// drainChannelsWaitingForNextTick continually closes any channels that are added to the
// addChannelWaitingForNextTick channel. This is used when the engine is shut down; it ensures
// any calls to WaitForNextTick that happen after a shutdown will not block.
func (w *World) drainChannelsWaitingForNextTick() {
	go func() {
		for ch := range w.addChannelWaitingForNextTick {
			close(ch)
		}
	}()
}

func (w *World) drainEndLoopChannels() {
	go func() {
		for range w.endGameLoopCh { //nolint:revive // This pattern drains the channel until closed
		}
	}()
}

// AddTransaction adds a transaction to the transaction queue. This should not be used directly.
// Instead, use a MessageType.AddToQueue to ensure type consistency. Returns the tick this transaction will be
// executed in.
func (w *World) AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (
	tick uint64, txHash types.TxHash,
) {
	// TODO: There's no locking between getting the tick and adding the transaction, so there's no guarantee that this
	// transaction is actually added to the returned tick.
	tick = w.CurrentTick()
	txHash = w.txQueue.AddTransaction(id, v, sig)
	return tick, txHash
}

func (w *World) AddEVMTransaction(
	id types.MessageID,
	v any,
	sig *sign.Transaction,
	evmTxHash string,
) (
	tick uint64, txHash types.TxHash,
) {
	tick = w.CurrentTick()
	txHash = w.txQueue.AddEVMTransaction(id, v, sig, evmTxHash)
	return tick, txHash
}

func (w *World) UseNonce(signerAddress string, nonce uint64) error {
	return w.redisStorage.UseNonce(signerAddress, nonce)
}

func (w *World) LoadGameState() error {
	if w.WorldState != WorldStateInit {
		return eris.New("cannot load game state multiple times")
	}

	// TODO(scott): footgun. so confusing.
	if err := w.entityStore.RegisterComponents(w.registeredComponents); err != nil {
		w.entityStore.Close()
		return err
	}

	recoveredTxs, err := w.recoverGameState()
	if err != nil {
		return err
	}

	// Engine is now ready to run
	w.WorldState = WorldStateReady

	// Recover the last tick, not to be confused with RecoverFromChain
	// It's ambigious, but screw it for now.
	if recoveredTxs != nil {
		w.txQueue = recoveredTxs
		if err = w.Tick(context.Background()); err != nil {
			return err
		}
	}
	w.receiptHistory.SetTick(w.CurrentTick())

	return nil
}

func (w *World) Namespace() Namespace {
	return w.namespace
}

func (w *World) GetQueryByName(name string) (engine.Query, error) {
	if q, ok := w.nameToQuery[name]; ok {
		return q, nil
	}
	return nil, eris.Errorf("query with name %s not found", name)
}

func (w *World) GameStateManager() gamestate.Manager {
	return w.entityStore
}

// WaitForNextTick blocks until at least one game tick has completed. It returns true if it successfully waited for a
// tick. False may be returned if the engine was shut down while waiting for the next tick to complete.
func (w *World) WaitForNextTick() (success bool) {
	startTick := w.CurrentTick()
	ch := make(chan struct{})
	w.addChannelWaitingForNextTick <- ch
	<-ch
	return w.CurrentTick() > startTick
}

func (w *World) GetEventHub() *events.EventHub {
	return w.eventHub
}

func (w *World) InjectLogger(logger *zerolog.Logger) {
	w.Logger = logger
	w.GameStateManager().InjectLogger(logger)
}

func (w *World) GetComponents() []types.ComponentMetadata {
	return w.registeredComponents
}

func (w *World) GetComponentByName(name string) (types.ComponentMetadata, error) {
	componentType, exists := w.nameToComponent[name]
	if !exists {
		return nil, eris.Wrapf(
			iterators.ErrMustRegisterComponent,
			"component %q must be registered before being used", name)
	}
	return componentType, nil
}

func (w *World) GetSystemNames() []string {
	return w.systemManager.GetSystemNames()
}
