package cardinal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"pkg.world.dev/world-engine/cardinal/component"
	"pkg.world.dev/world-engine/cardinal/gamestate"
	ecslog "pkg.world.dev/world-engine/cardinal/log"
	"pkg.world.dev/world-engine/cardinal/message"
	"pkg.world.dev/world-engine/cardinal/query"
	"pkg.world.dev/world-engine/cardinal/receipt"
	"pkg.world.dev/world-engine/cardinal/router"
	"pkg.world.dev/world-engine/cardinal/search"
	"pkg.world.dev/world-engine/cardinal/search/filter"
	"pkg.world.dev/world-engine/cardinal/server"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/storage/redis"
	"pkg.world.dev/world-engine/cardinal/system"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"pkg.world.dev/world-engine/cardinal/types/txpool"
	"pkg.world.dev/world-engine/cardinal/worldstage"
	"pkg.world.dev/world-engine/sign"
)

const (
	DefaultHistoricalTicksToStore = 10
	RedisDialTimeOut              = 15
)

var _ router.Provider = &World{}      //nolint:exhaustruct
var _ servertypes.Provider = &World{} //nolint:exhaustruct

type World struct {
	mode      RunMode
	namespace Namespace

	// Storage
	redisStorage *redis.Storage
	entityStore  gamestate.Manager

	// Networking
	server        *server.Server
	serverOptions []server.Option

	// Core modules
	worldStage       *worldstage.Manager
	msgManager       *message.Manager
	systemManager    *system.Manager
	componentManager *component.Manager
	queryManager     *query.Manager
	router           router.Router
	txPool           *txpool.TxPool

	// Receipt
	receiptHistory *receipt.History
	evmTxReceipts  map[string]EVMTxReceipt

	// Tick
	tick            *atomic.Uint64
	timestamp       *atomic.Uint64
	tickResults     *TickResults
	tickChannel     <-chan time.Time
	tickDoneChannel chan<- uint64
	// addChannelWaitingForNextTick accepts a channel which will be closed after a tick has been completed.
	addChannelWaitingForNextTick chan chan struct{}
}

// NewWorld creates a new World object using Redis as the storage layer
func NewWorld(opts ...WorldOption) (*World, error) {
	serverOptions, cardinalOptions := separateOptions(opts)

	// Load config. Fallback value is used if it's not set.
	cfg, err := loadWorldConfig()
	if err != nil {
		return nil, eris.Wrap(err, "Failed to load config to start world")
	}

	log.Info().Msgf("Creating a new Cardinal world in %s mode", cfg.CardinalMode)

	redisMetaStore := redis.NewRedisStorage(redis.Options{
		Addr:        cfg.RedisAddress,
		Password:    cfg.RedisPassword,
		DB:          0,                              // use default DB
		DialTimeout: RedisDialTimeOut * time.Second, // Increase startup dial timeout
	}, cfg.CardinalNamespace)

	redisStore := gamestate.NewRedisPrimitiveStorage(redisMetaStore.Client)
	entityCommandBuffer, err := gamestate.NewEntityCommandBuffer(&redisStore)
	if err != nil {
		return nil, err
	}

	tick := new(atomic.Uint64)

	world := &World{
		mode:      cfg.CardinalMode,
		namespace: Namespace(cfg.CardinalNamespace),

		// Storage
		redisStorage: &redisMetaStore,
		entityStore:  entityCommandBuffer,

		// Networking
		server:        nil, // Will be initialized in StartGame
		serverOptions: serverOptions,

		// Core modules
		worldStage:       worldstage.NewManager(),
		msgManager:       message.NewManager(),
		systemManager:    system.NewManager(),
		componentManager: component.NewManager(&redisMetaStore),
		queryManager:     query.NewManager(),
		router:           nil, // Will be set if run mode is production or its injected via options
		txPool:           txpool.New(),

		// Receipt
		receiptHistory: receipt.NewHistory(tick.Load(), DefaultHistoricalTicksToStore),
		evmTxReceipts:  make(map[string]EVMTxReceipt),

		// Tick
		tick:                         tick,
		timestamp:                    new(atomic.Uint64),
		tickResults:                  NewTickResults(tick.Load()),
		tickChannel:                  time.Tick(time.Second), //nolint:staticcheck // its ok.
		tickDoneChannel:              nil,                    // Will be injected via options
		addChannelWaitingForNextTick: make(chan chan struct{}),
	}

	// Shard router must be set in production mode
	if cfg.CardinalMode == RunModeProd {
		world.router, err = router.New(cfg.CardinalNamespace, cfg.BaseShardSequencerAddress, cfg.BaseShardQueryAddress,
			world)
		if err != nil {
			return nil, eris.Wrap(err, "Failed to initialize shard router")
		}
	}

	// Apply options
	for _, opt := range cardinalOptions {
		opt(world)
	}

	world.registerInternalPlugin()

	var metricTags []string
	metricTags = append(metricTags, string("cardinal_mode:"+cfg.CardinalMode))
	metricTags = append(metricTags, "cardinal_namespace:"+cfg.CardinalNamespace)

	if cfg.StatsdAddress != "" || cfg.TraceAddress != "" {
		if err = statsd.Init(cfg.StatsdAddress, cfg.TraceAddress, metricTags); err != nil {
			return nil, eris.Wrap(err, "unable to init statsd")
		}
	} else {
		log.Logger.Warn().Msg("statsd is disabled")
	}

	return world, nil
}

func (w *World) CurrentTick() uint64 {
	return w.tick.Load()
}

// doTick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (w *World) doTick(ctx context.Context, timestamp uint64) (err error) {
	// Record tick start time for statsd.
	// Not to be confused with `timestamp` that represents the time context for the tick
	// that is injected into system via WorldContext.Timestamp() and recorded into the DA.
	startTime := time.Now()

	// The world can only perform a tick if:
	// - We're in a recovery tick
	// - The world is currently running
	// - The world is shutting down (this will be the last or penultimate tick)
	if w.worldStage.Current() != worldstage.Recovering &&
		w.worldStage.Current() != worldstage.Running &&
		w.worldStage.Current() != worldstage.ShuttingDown {
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

	log.Info().Int("tick", int(w.CurrentTick())).Msg("Tick started")

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

	finalizeTickStartTime := time.Now()
	if err := w.entityStore.FinalizeTick(ctx); err != nil {
		return err
	}
	statsd.EmitTickStat(finalizeTickStartTime, "finalize")

	w.setEvmResults(txPool.GetEVMTxs())

	// Handle tx data blob submission
	// Only submit transactions when the following criteria is satisfied:
	// 1. The shard router is set
	// 2. The world is not in the recovering stage (we don't want to resubmit past transactions)
	if w.router != nil && w.worldStage.Current() != worldstage.Recovering {
		err := w.router.SubmitTxBlob(ctx, txPool.Transactions(), w.tick.Load(), w.timestamp.Load())
		if err != nil {
			return fmt.Errorf("failed to submit transactions to base shard: %w", err)
		}
	}

	// Increment the tick
	w.tick.Add(1)
	w.receiptHistory.NextTick() // todo(scott): use channels

	// Populate world.TickResults for the current tick and emit it as an Event
	flushEventStart := time.Now()
	w.populateAndBroadcastTickResults()
	statsd.EmitTickStat(flushEventStart, "flush_events")

	// Clear the TickResults for this tick in preparation for the next Tick
	w.tickResults.Clear()

	statsd.EmitTickStat(startTime, "full_tick")
	if err := statsd.Client().Count("num_of_txs", int64(txPool.GetAmountOfTxs()), nil, 1); err != nil {
		log.Warn().Msgf("failed to emit count stat:%v", err)
	}

	return nil
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
		if err := w.router.RegisterGameShard(context.Background()); err != nil {
			return eris.Wrap(err, "failed to register game shard to base shard")
		}
	}

	w.worldStage.Store(worldstage.Recovering)
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
	w.worldStage.Store(worldstage.Ready)

	// TODO(scott): i find this manual tracking and incrementing of the tick very footgunny. Why can't we just
	//  use a reliable source of truth for the tick? It's not clear to me why we need to manually increment the
	//  receiptHistory tick separately.
	w.receiptHistory.SetTick(w.CurrentTick())

	// Create server
	// We can't do this is in NewWorld() because the server needs to know the registered messages
	// and register queries first. We can probably refactor this though.
	w.server, err = server.New(w,
		NewReadOnlyWorldContext(w), w.GetRegisteredComponents(), w.GetRegisteredMessages(),
		w.GetRegisteredQueries(), w.serverOptions...)
	if err != nil {
		return err
	}

	// Warn when no components, messages, queries, or systems are registered
	if len(w.componentManager.GetComponents()) == 0 {
		log.Warn().Msg("No components registered")
	}
	if len(w.msgManager.GetRegisteredMessages()) == 0 {
		log.Warn().Msg("No messages registered")
	}
	if len(w.queryManager.GetRegisteredQueries()) == 0 {
		log.Warn().Msg("No queries registered")
	}
	if len(w.systemManager.GetRegisteredSystemNames()) == 0 {
		log.Warn().Msg("No systems registered")
	}

	// Log world info
	ecslog.World(&log.Logger, w, zerolog.InfoLevel)

	// Game stage: Ready -> Running
	w.worldStage.Store(worldstage.Running)

	// Start the game loop
	w.startGameLoop(context.Background(), w.tickChannel, w.tickDoneChannel)

	// Start the server
	w.startServer()

	// handle shutdown via a signal
	w.handleShutdown()
	<-w.worldStage.NotifyOnStage(worldstage.ShutDown)
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
	log.Info().Msg("Game loop started")
	go func() {
		var waitingChs []chan struct{}
	loop:
		for {
			select {
			case _, ok := <-tickStart:
				if !ok {
					panic("tickStart channel has been closed; tick rate is now unbounded.")
				}
				w.tickTheEngine(ctx, tickDone)
				closeAllChannels(waitingChs)
				waitingChs = waitingChs[:0]
			case <-w.worldStage.NotifyOnStage(worldstage.ShuttingDown):
				w.drainChannelsWaitingForNextTick()
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
		w.worldStage.Store(worldstage.ShutDown)
	}()
}

func (w *World) tickTheEngine(ctx context.Context, tickDone chan<- uint64) {
	currTick := w.CurrentTick()
	// this is the final point where errors bubble up and hit a panic. There are other places where this occurs
	// but this is the highest terminal point.
	// the panic may point you to here, (or the tick function) but the real stack trace is in the error message.
	err := w.doTick(ctx, uint64(time.Now().Unix()))
	if err != nil {
		bytes, errMarshal := json.Marshal(eris.ToJSON(err, true))
		if errMarshal != nil {
			panic(errMarshal)
		}
		panic(string(bytes))
	}
	if tickDone != nil {
		tickDone <- currTick
	}
}

func (w *World) IsGameRunning() bool {
	return w.worldStage.Current() == worldstage.Running
}

func (w *World) Shutdown() error {
	log.Info().Msg("Shutting down game loop.")
	ok := w.worldStage.CompareAndSwap(worldstage.Running, worldstage.ShuttingDown)
	if !ok {
		select {
		case <-w.worldStage.NotifyOnStage(worldstage.ShuttingDown):
			// Some other goroutine has already started the shutdown process. Wait until the world is
			// actually shut down.
			<-w.worldStage.NotifyOnStage(worldstage.ShutDown)
			return nil
		default:
		}
		return errors.New("shutdown attempted before the world was started")
	}

	// Block until the world has stopped ticking
	<-w.worldStage.NotifyOnStage(worldstage.ShutDown)

	if w.server != nil {
		if err := w.server.Shutdown(); err != nil {
			return err
		}
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
		log.Error().Msgf(
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

func (w *World) Namespace() string {
	return string(w.namespace)
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

func (w *World) Search(filter filter.ComponentFilter) *search.Search {
	return NewSearch(NewReadOnlyWorldContext(w), filter)
}

func (w *World) StoreReader() gamestate.Reader {
	return w.entityStore.ToReadOnly()
}

func (w *World) GetRegisteredQueries() []engine.Query {
	return w.queryManager.GetRegisteredQueries()
}
func (w *World) GetRegisteredMessages() []types.Message {
	return w.msgManager.GetRegisteredMessages()
}

func (w *World) GetRegisteredComponents() []types.ComponentMetadata {
	return w.componentManager.GetComponents()
}
func (w *World) GetRegisteredSystemNames() []string {
	return w.systemManager.GetRegisteredSystemNames()
}

func (w *World) GetQueryByName(name string) (engine.Query, error) {
	return w.queryManager.GetQueryByName(name)
}

func (w *World) GetMessageByID(id types.MessageID) (types.Message, bool) {
	msg := w.msgManager.GetMessageByID(id)
	return msg, msg != nil
}

func (w *World) GetMessageByFullName(name string) (types.Message, bool) {
	return w.msgManager.GetMessageByFullName(name)
}

func (w *World) GetComponentByName(name string) (types.ComponentMetadata, error) {
	return w.componentManager.GetComponentByName(name)
}

func (w *World) populateAndBroadcastTickResults() {
	receipts, err := w.receiptHistory.GetReceiptsForTick(w.CurrentTick() - 1)
	if err != nil {
		log.Error().Err(err).Msgf("failed get receipts for tick %d", w.CurrentTick())
	}
	w.tickResults.SetReceipts(receipts)
	w.tickResults.SetTick(w.CurrentTick() - 1)

	// Broadcast the tick results to all clients
	err = w.server.BroadcastEvent(w.tickResults)
	if err != nil {
		log.Err(err).Msgf("failed to broadcast tick results")
	}
}
