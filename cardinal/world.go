package cardinal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/rs/zerolog"

	"pkg.world.dev/world-engine/cardinal/component"
	"pkg.world.dev/world-engine/cardinal/events"
	ecslog "pkg.world.dev/world-engine/cardinal/log"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/router"
	"pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/system"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	"pkg.world.dev/world-engine/sign"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	"pkg.world.dev/world-engine/cardinal/server"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/worldstage"
)

const (
	DefaultHistoricalTicksToStore = 10
)

type World struct {
	server          *server.Server
	tickChannel     <-chan time.Time
	tickDoneChannel chan<- uint64
	serverOptions   []server.Option
	cleanup         func()
	mode            RunMode
	Logger          *zerolog.Logger

	endStartGame chan bool

	worldStage       *worldstage.Manager
	msgManager       *message.Manager
	systemManager    *system.Manager
	componentManager *component.Manager

	namespace         Namespace
	redisStorage      *redis.Storage
	entityStore       gamestate.Manager
	tick              *atomic.Uint64
	timestamp         *atomic.Uint64
	nameToQuery       map[string]engine.Query
	registeredQueries []engine.Query

	evmTxReceipts map[string]EVMTxReceipt

	txPool *txpool.TxPool

	receiptHistory *receipt.History

	router router.Router

	endGameLoopCh     chan bool
	isGameLoopRunning atomic.Bool

	eventHub *events.EventHub

	// addChannelWaitingForNextTick accepts a channel which will be closed after a tick has been completed.
	addChannelWaitingForNextTick chan chan struct{}

	shutdownMutex sync.Mutex
}

var _ router.Provider = &World{}

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
		serverOptions = append(serverOptions, server.WithPrettyPrint())
	}
	redisMetaStore := redis.NewRedisStorage(redis.Options{
		Addr:     cfg.RedisAddress,
		Password: cfg.RedisPassword,
		DB:       0, // use default DB
	}, cfg.CardinalNamespace)

	redisStore := gamestate.NewRedisPrimitiveStorage(redisMetaStore.Client)

	entityCommandBuffer, err := gamestate.NewEntityCommandBuffer(&redisStore)
	if err != nil {
		return nil, err
	}

	world := &World{
		serverOptions: serverOptions,
		mode:          cfg.CardinalMode,
		endStartGame:  make(chan bool),

		worldStage:       worldstage.NewManager(),
		msgManager:       message.NewManager(),
		systemManager:    system.NewManager(),
		componentManager: component.NewManager(&redisMetaStore),

		// Imported from engine
		redisStorage:      &redisMetaStore,
		entityStore:       entityCommandBuffer,
		namespace:         Namespace(cfg.CardinalNamespace),
		tick:              &atomic.Uint64{},
		timestamp:         new(atomic.Uint64),
		nameToQuery:       make(map[string]engine.Query),
		txPool:            txpool.New(),
		Logger:            &log.Logger,
		isGameLoopRunning: atomic.Bool{},
		endGameLoopCh:     make(chan bool),
		evmTxReceipts:     make(map[string]EVMTxReceipt),

		addChannelWaitingForNextTick: make(chan chan struct{}),
	}

	if cfg.CardinalMode == RunModeProd {
		world.router, err = router.New(cfg.CardinalNamespace, cfg.BaseShardSequencerAddress, cfg.BaseShardQueryAddress,
			world)
		if err != nil {
			return nil, err
		}
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

	// Make game loop tick every second if not set
	if world.tickChannel == nil {
		world.tickChannel = time.Tick(time.Second) //nolint:staticcheck // its ok.
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

func GetMessageFromWorld[In any, Out any](world *World) (*message.MessageType[In, Out], error) {
	var msg message.MessageType[In, Out]
	msgType := reflect.TypeOf(msg)
	tempRes, ok := world.GetMessageManager().GetMessageByType(msgType)
	if !ok {
		return &msg, eris.Errorf("Could not find %s, Message may not be registered.", msg.Name())
	}
	var _ types.Message = &msg
	res, ok := tempRes.(*message.MessageType[In, Out])
	if !ok {
		return &msg, eris.New("wrong type")
	}
	return res, nil
}

func (w *World) CurrentTick() uint64 {
	return w.tick.Load()
}

// TODO(scott): we should make Tick() private and only allow controlling tick externally using the tick channel.
//  This is a footgun because you want to make sure world is started using StartWorld() instead of calling Tick()
//  directly. This is unfortunately used a lot in tests.
//  One thing we need to do is to make the tickDone channel return the tick number AND error if any so it can be used
//  to check if there are errors when trying to tick.

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (w *World) Tick(ctx context.Context, timestamp uint64) error {
	// Record tick start time for statsd.
	// Not to be confused with `timestamp` that represents the time context for the tick
	// that is injected into system via WorldContext.Timestamp() and recorded into the DA.
	startTime := time.Now()

	// The world can only start ticking if it's in the running or recovering stage.
	if w.worldStage.Current() != worldstage.Running && w.worldStage.Current() != worldstage.Recovering {
		return eris.Errorf("invalid world state to tick: %s", w.worldStage.Current())
	}

	// This defer is here to catch any panics that occur during the tick. It will log the current tick and the
	// current system that is running.
	defer w.handleTickPanic()

	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "cardinal.span.tick")
	defer func() {
		span.Finish()
	}()

	w.Logger.Info().Int("tick", int(w.CurrentTick())).Msg("Tick started")

	// Copy the transactions from the pool so that we can safely modify the pool while the tick is running.
	txPool := w.txPool.CopyTransactions()

	if err := w.entityStore.StartNextTick(w.msgManager.GetRegisteredMessages(), txPool); err != nil {
		return err
	}

	// Store the timestamp for this tick
	w.timestamp.Store(timestamp)

	// Create the engine context to inject into systems
	wCtx := newWorldContextForTick(w, txPool)

	// Run all registered systems.
	// This will run the registered init systems if the current tick is 0
	if err := w.systemManager.RunSystems(wCtx); err != nil {
		return err
	}

	if w.eventHub != nil {
		// engine can be optionally loaded with or without an eventHub.
		// If there is one, on every tick it must flush events.
		flushEventStart := time.Now()
		w.eventHub.FlushEvents()
		statsd.EmitTickStat(flushEventStart, "flush_events")
	}

	finalizeTickStartTime := time.Now()
	if err := w.entityStore.FinalizeTick(ctx); err != nil {
		return err
	}
	statsd.EmitTickStat(finalizeTickStartTime, "finalize")

	w.setEvmResults(txPool.GetEVMTxs())

	// Handle tx data blob submission
	// Only submit transactions when the following criteria is satisfied:
	// 1. There are transactions in the pool
	// 2. The shard router is set
	// 3. The world is not in the recovering stage (we don't want to resubmit past transactions)
	if txPool.GetAmountOfTxs() != 0 && w.router != nil && w.worldStage.Current() != worldstage.Recovering {
		err := w.router.SubmitTxBlob(ctx, txPool.Transactions(), w.tick.Load(), w.timestamp.Load())
		if err != nil {
			return fmt.Errorf("failed to submit transactions to base shard: %w", err)
		}
	}

	// Increment the tick
	w.tick.Add(1)
	w.receiptHistory.NextTick() // todo(scott): use channels

	statsd.EmitTickStat(startTime, "full_tick")
	if err := statsd.Client().Count("num_of_txs", int64(txPool.GetAmountOfTxs()), nil, 1); err != nil {
		w.Logger.Warn().Msgf("failed to emit count stat:%v", err)
	}

	return nil
}

func (w *World) GetMessageManager() *message.Manager {
	return w.msgManager
}

// StartGame starts running the world game loop. Each time a message arrives on the tickChannel, a world tick is
// attempted. In addition, an HTTP server (listening on the given port) is created so that game messages can be sent
// to this world. After StartGame is called, RegisterComponent, registerMessagesByName,
// RegisterQueries, and RegisterSystems may not be called. If StartGame doesn't encounter any errors, it will
// block forever, running the server and ticking the game in the background.
func (w *World) StartGame() error {
	// Game stage: Init -> Starting
	ok := w.worldStage.CompareAndSwap(worldstage.Init, worldstage.Starting)
	if !ok {
		return errors.New("game has already been started")
	}

	// TODO(scott): entityStore.RegisterComponents is ambiguous with cardinal.RegisterComponent.
	//  We should probably rename this to LoadComponents or osmething.
	if err := w.entityStore.RegisterComponents(w.componentManager.GetComponents()); err != nil {
		closeErr := w.entityStore.Close()
		if closeErr != nil {
			return eris.Wrap(err, closeErr.Error())
		}
		return err
	}

	// Start router if it is set
	if w.router != nil {
		if err := w.router.Start(); err != nil {
			return eris.Wrap(err, "failed to start router service")
		}
	}

	// Recover pending transactions from redis
	err := w.recoverAndExecutePendingTxs()
	if err != nil {
		return err
	}

	// If Cardinal is in Prod and Router is set, recover any old state of the engine from the chain
	if w.mode == RunModeProd && w.router != nil {
		if err := w.RecoverFromChain(context.Background()); err != nil {
			return eris.Wrap(err, "failed to recover from chain")
		}
	}

	// TODO(scott): i find this manual tracking and incrementing of the tick very footgunny. Why can't we just
	//  use a reliable source of truth for the tick? It's not clear to me why we need to manually increment the
	//  receiptHistory tick separately.
	w.receiptHistory.SetTick(w.CurrentTick())

	// Create server
	// We can't do this is in NewWorld() because the server needs to know the registered messages
	// and register queries first. We can probably refactor this though.
	w.server, err = server.New(NewReadOnlyWorldContext(w), w.ListMessages(), w.ListQueries(),
		w.eventHub.NewWebSocketEventHandler(),
		w.serverOptions...)
	if err != nil {
		return err
	}

	// Game stage: Starting -> Running
	w.worldStage.CompareAndSwap(worldstage.Starting, worldstage.Running)

	// Start the game loop
	w.startGameLoop(context.Background(), w.tickChannel, w.tickDoneChannel)

	// Start the server
	w.startServer()

	// handle shutdown via a signal
	w.handleShutdown()
	<-w.endStartGame
	return err
}

func (w *World) startServer() {
	go func() {
		if err := w.server.Serve(); errors.Is(err, http.ErrServerClosed) {
			log.Info().Err(err).Msgf("the server has been closed: %s", eris.ToString(err, true))
		} else if err != nil {
			log.Fatal().Err(err).Msgf("the server has failed: %s", eris.ToString(err, true))
		}
	}()
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
				if w.txPool.GetAmountOfTxs() > 0 {
					// immediately tick if pool is not empty to process all txs if queue is not empty.
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
	if err := w.Tick(ctx, uint64(time.Now().Unix())); err != nil {
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
	if len(w.componentManager.GetComponents()) == 0 {
		w.Logger.Warn().Msg("No components registered.")
	}
	if len(w.msgManager.GetRegisteredMessages()) == 0 {
		w.Logger.Warn().Msg("No messages registered.")
	}
	if len(w.registeredQueries) == 0 {
		w.Logger.Warn().Msg("No queries registered.")
	}
	if len(w.systemManager.GetRegisteredSystemNames()) == 0 {
		w.Logger.Warn().Msg("No systems registered.")
	}
}

func (w *World) IsGameRunning() bool {
	return w.worldStage.Current() == worldstage.Running
}

func (w *World) Shutdown() error {
	if w.cleanup != nil {
		w.cleanup()
	}
	w.worldStage.Store(worldstage.ShuttingDown)

	// The CompareAndSwap returned true, so this call is responsible for actually
	// shutting down the game.
	defer func() {
		w.worldStage.Store(worldstage.ShutDown)
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

func (w *World) handleTickPanic() {
	if r := recover(); r != nil {
		w.Logger.Error().Msgf(
			"Tick: %d, Current running system: %s",
			w.CurrentTick(),
			w.systemManager.GetCurrentSystem(),
		)
		panic(r)
	}
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

// AddTransaction adds a transaction to the transaction pool. This should not be used directly.
// Instead, use a MessageType.AddTransaction to ensure type consistency. Returns the tick this transaction will be
// executed in.
func (w *World) AddTransaction(id types.MessageID, v any, sig *sign.Transaction) (
	tick uint64, txHash types.TxHash,
) {
	// TODO: There's no locking between getting the tick and adding the transaction, so there's no guarantee that this
	// transaction is actually added to the returned tick.
	tick = w.CurrentTick()
	txHash = w.txPool.AddTransaction(id, v, sig)
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
	txHash = w.txPool.AddEVMTransaction(id, v, sig, evmTxHash)
	return tick, txHash
}

func (w *World) UseNonce(signerAddress string, nonce uint64) error {
	return w.redisStorage.UseNonce(signerAddress, nonce)
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

func (w *World) GetMessageByName(name string) (types.Message, bool) {
	return w.msgManager.GetMessageByName(name)
}

func (w *World) GetMessageByID(id types.MessageID) (types.Message, bool) {
	msg := w.msgManager.GetMessageByID(id)
	return msg, msg != nil
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

func (w *World) GetRegisteredComponents() []types.ComponentMetadata {
	return w.componentManager.GetComponents()
}

func (w *World) GetComponentByName(name string) (types.ComponentMetadata, error) {
	return w.componentManager.GetComponentByName(name)
}

func (w *World) GetRegisteredSystemNames() []string {
	return w.systemManager.GetRegisteredSystemNames()
}

func (w *World) SetRouter(rtr router.Router) {
	w.router = rtr
}

func (w *World) HandleEVMQuery(name string, abiRequest []byte) ([]byte, error) {
	qry, err := w.GetQueryByName(name)
	if err != nil {
		return nil, err
	}
	req, err := qry.DecodeEVMRequest(abiRequest)
	if err != nil {
		return nil, err
	}

	reply, err := qry.HandleQuery(NewReadOnlyWorldContext(w), req)
	if err != nil {
		return nil, err
	}

	return qry.EncodeEVMReply(reply)
}
