package ecs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/ecs/filter"
	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/storage/redis"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/evm/x/shard/types"
	shardv1 "pkg.world.dev/world-engine/rift/shard/v1"
	"pkg.world.dev/world-engine/sign"
)

// Namespace is a unique identifier for a engine.
type Namespace string

func (n Namespace) String() string {
	return string(n)
}

type Engine struct {
	namespace              Namespace
	redisStorage           *redis.Storage
	entityStore            gamestate.Manager
	systems                []System
	systemLoggers          []*zerolog.Logger
	initSystem             System
	initSystemLogger       *zerolog.Logger
	systemNames            []string
	tick                   *atomic.Uint64
	timestamp              *atomic.Uint64
	nameToComponent        map[string]component.ComponentMetadata
	nameToQuery            map[string]Query
	registeredComponents   []component.ComponentMetadata
	registeredMessages     []message.Message
	registeredQueries      []Query
	isComponentsRegistered bool
	isEntitiesCreated      bool
	isMessagesRegistered   bool
	stateIsLoaded          bool

	evmTxReceipts map[string]EVMTxReceipt

	txQueue *txpool.TxQueue

	receiptHistory *receipt.History

	chain shard.Adapter
	// isRecovering indicates that the engine is recovering from the DA layer.
	// this is used to prevent ticks from submitting duplicate transactions the DA layer.
	isRecovering atomic.Bool

	Logger *zerolog.Logger

	endGameLoopCh     chan bool
	isGameLoopRunning atomic.Bool

	nextComponentID component.TypeID

	eventHub events.EventHub

	// addChannelWaitingForNextTick accepts a channel which will be closed after a tick has been completed.
	addChannelWaitingForNextTick chan chan struct{}

	shutdownMutex sync.Mutex
}

var (
	ErrEntitiesCreatedBeforeLoadingGameState = errors.New("cannot create entities before loading game state")
	ErrMessageRegistrationMustHappenOnce     = errors.New(
		"message registration must happen exactly 1 time",
	)
	ErrStoreStateInvalid    = errors.New("saved engine state is not valid")
	ErrDuplicateMessageName = errors.New("message names must be unique")
	ErrDuplicateQueryName   = errors.New("query names must be unique")
)

const (
	defaultReceiptHistorySize = 10
)

func (e *Engine) DoesEngineHaveAnEventHub() bool {
	return e.eventHub != nil
}

func (e *Engine) GetEventHub() events.EventHub {
	return e.eventHub
}

func (e *Engine) IsEntitiesCreated() bool {
	return e.isEntitiesCreated
}

func (e *Engine) SetEntitiesCreated(value bool) {
	e.isEntitiesCreated = value
}

func (e *Engine) SetEventHub(eventHub events.EventHub) {
	e.eventHub = eventHub
}

func (e *Engine) EmitEvent(event *events.Event) {
	e.eventHub.EmitEvent(event)
}

func (e *Engine) FlushEvents() {
	e.eventHub.FlushEvents()
}

func (e *Engine) IsRecovering() bool {
	return e.isRecovering.Load()
}

func (e *Engine) Namespace() Namespace {
	return e.namespace
}

func (e *Engine) GameStateManager() gamestate.Manager {
	return e.entityStore
}

func (e *Engine) TickStore() gamestate.TickStorage {
	return e.entityStore
}

func (e *Engine) GetTxQueueAmount() int {
	return e.txQueue.GetAmountOfTxs()
}

func (e *Engine) RegisterSystem(s System) {
	e.RegisterSystemWithName(s, "")
}

func (e *Engine) RegisterSystems(systems ...System) {
	for _, system := range systems {
		e.RegisterSystemWithName(system, "")
	}
}

func (e *Engine) RegisterSystemWithName(system System, functionName string) {
	if e.stateIsLoaded {
		panic("cannot register systems after loading game state")
	}
	if functionName == "" {
		functionName = filepath.Base(runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name())
	}
	sysLogger := ecslog.CreateSystemLogger(e.Logger, functionName)
	e.systemLoggers = append(e.systemLoggers, sysLogger)
	e.systemNames = append(e.systemNames, functionName)
	// appends registeredSystem into the member system list in engine.
	e.systems = append(e.systems, system)
	e.checkDuplicateSystemName()
}

func (e *Engine) checkDuplicateSystemName() {
	mappedNames := make(map[string]int, len(e.systemNames))
	for _, sysName := range e.systemNames {
		if sysName != "" {
			mappedNames[sysName]++
			if mappedNames[sysName] > 1 {
				e.Logger.Warn().Msgf("duplicate system registered: %s", sysName)
			}
		}
	}
}

func (e *Engine) AddInitSystem(system System) {
	logger := ecslog.CreateSystemLogger(e.Logger, "InitSystem")
	e.initSystemLogger = logger
	e.initSystem = system
}

func RegisterComponent[T component.Component](engine *Engine) error {
	if engine.stateIsLoaded {
		panic("cannot register components after loading game state")
	}
	var t T
	_, err := engine.GetComponentByName(t.Name())
	if err == nil {
		return eris.Errorf("component with name '%s' is already registered", t.Name())
	}
	c, err := component.NewComponentMetadata[T]()
	if err != nil {
		return err
	}
	err = c.SetID(engine.nextComponentID)
	if err != nil {
		return err
	}
	engine.registeredComponents = append(engine.registeredComponents, c)

	storedSchema, err := engine.redisStorage.GetSchema(c.Name())

	if err != nil {
		// It's fine if the schema doesn't currently exist in the db. Any other errors are a problem.
		if !eris.Is(err, redis.ErrNoSchemaFound) {
			return err
		}
	} else {
		valid, err := component.IsComponentValid(t, storedSchema)
		if err != nil {
			return err
		}
		if !valid {
			return eris.Errorf("Component: %s does not match the type stored in the db", c.Name())
		}
	}

	err = engine.redisStorage.SetSchema(c.Name(), c.GetSchema())
	if err != nil {
		return err
	}
	engine.nextComponentID++
	engine.nameToComponent[t.Name()] = c
	engine.isComponentsRegistered = true
	return nil
}

func MustRegisterComponent[T component.Component](engine *Engine) {
	err := RegisterComponent[T](engine)
	if err != nil {
		panic(err)
	}
}

func (e *Engine) GetComponentByName(name string) (component.ComponentMetadata, error) {
	componentType, exists := e.nameToComponent[name]
	if !exists {
		return nil, eris.Wrapf(
			storage.ErrMustRegisterComponent,
			"component %q must be registered before being used", name)
	}
	return componentType, nil
}

func RegisterQuery[Request any, Reply any](
	engine *Engine,
	name string,
	handler func(eCtx EngineContext, req *Request) (*Reply, error),
	opts ...func() func(queryType *QueryType[Request, Reply]),
) error {
	if engine.stateIsLoaded {
		panic("cannot register queries after loading game state")
	}

	if _, ok := engine.nameToQuery[name]; ok {
		return eris.Errorf("query with name %s is already registered", name)
	}

	q, err := NewQueryType[Request, Reply](name, handler, opts...)
	if err != nil {
		return err
	}

	engine.registeredQueries = append(engine.registeredQueries, q)
	engine.nameToQuery[q.Name()] = q

	return nil
}

func (e *Engine) GetQueryByName(name string) (Query, error) {
	if q, ok := e.nameToQuery[name]; ok {
		return q, nil
	}
	return nil, eris.Errorf("query with name %s not found", name)
}

func (e *Engine) RegisterMessages(txs ...message.Message) error {
	if e.stateIsLoaded {
		panic("cannot register messages after loading game state")
	}
	if e.isMessagesRegistered {
		return eris.Wrap(ErrMessageRegistrationMustHappenOnce, "")
	}
	e.isMessagesRegistered = true
	e.registerInternalMessages()
	e.registeredMessages = append(e.registeredMessages, txs...)

	seenTxNames := map[string]bool{}
	for i, t := range e.registeredMessages {
		name := t.Name()
		if seenTxNames[name] {
			return eris.Wrapf(ErrDuplicateMessageName, "duplicate tx %q", name)
		}
		seenTxNames[name] = true

		id := message.TypeID(i + 1)
		if err := t.SetID(id); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) registerInternalMessages() {
	e.registeredMessages = append(
		e.registeredMessages,
		CreatePersonaMsg,
		AuthorizePersonaAddressMsg,
	)
}

func (e *Engine) ListQueries() []Query {
	return e.registeredQueries
}

func (e *Engine) ListMessages() ([]message.Message, error) {
	if !e.isMessagesRegistered {
		return nil, eris.New("cannot list messages until message registration occurs")
	}
	return e.registeredMessages, nil
}

// NewEngine creates a new engine.
func NewEngine(
	storage *redis.Storage,
	entityStore gamestate.Manager,
	namespace Namespace,
	opts ...Option,
) (*Engine, error) {
	logger := &log.Logger
	entityStore.InjectLogger(logger)
	e := &Engine{
		redisStorage:      storage,
		entityStore:       entityStore,
		namespace:         namespace,
		tick:              &atomic.Uint64{},
		timestamp:         new(atomic.Uint64),
		systems:           make([]System, 0),
		initSystem:        func(_ EngineContext) error { return nil },
		nameToComponent:   make(map[string]component.ComponentMetadata),
		nameToQuery:       make(map[string]Query),
		txQueue:           txpool.NewTxQueue(),
		Logger:            logger,
		isGameLoopRunning: atomic.Bool{},
		isEntitiesCreated: false,
		endGameLoopCh:     make(chan bool),
		nextComponentID:   1,
		evmTxReceipts:     make(map[string]EVMTxReceipt),

		addChannelWaitingForNextTick: make(chan chan struct{}),
	}
	e.isGameLoopRunning.Store(false)
	e.RegisterSystems(RegisterPersonaSystem, AuthorizePersonaAddressSystem)
	err := RegisterComponent[SignerComponent](e)
	if err != nil {
		return nil, err
	}
	opts = append([]Option{WithEventHub(events.NewWebSocketEventHub())}, opts...)
	for _, opt := range opts {
		opt(e)
	}
	if e.receiptHistory == nil {
		e.receiptHistory = receipt.NewHistory(e.CurrentTick(), defaultReceiptHistorySize)
	}
	return e, nil
}

func (e *Engine) CurrentTick() uint64 {
	return e.tick.Load()
}

func (e *Engine) ReceiptHistorySize() uint64 {
	return e.receiptHistory.Size()
}

// Remove removes the given Entity from the engine.
func (e *Engine) Remove(id entity.ID) error {
	return e.GameStateManager().RemoveEntity(id)
}

// ConsumeEVMMsgResult consumes a tx result from an EVM originated Cardinal message.
// It will fetch the receipt from the map, and then delete ('consume') it from the map.
func (e *Engine) ConsumeEVMMsgResult(evmTxHash string) (EVMTxReceipt, bool) {
	r, ok := e.evmTxReceipts[evmTxHash]
	delete(e.evmTxReceipts, evmTxHash)
	return r, ok
}

// AddTransaction adds a transaction to the transaction queue. This should not be used directly.
// Instead, use a MessageType.AddToQueue to ensure type consistency. Returns the tick this transaction will be
// executed in.
func (e *Engine) AddTransaction(id message.TypeID, v any, sig *sign.Transaction) (
	tick uint64, txHash message.TxHash,
) {
	// TODO: There's no locking between getting the tick and adding the transaction, so there's no guarantee that this
	// transaction is actually added to the returned tick.
	tick = e.CurrentTick()
	txHash = e.txQueue.AddTransaction(id, v, sig)
	return tick, txHash
}

func (e *Engine) AddEVMTransaction(
	id message.TypeID,
	v any,
	sig *sign.Transaction,
	evmTxHash string,
) (
	tick uint64, txHash message.TxHash,
) {
	tick = e.CurrentTick()
	txHash = e.txQueue.AddEVMTransaction(id, v, sig, evmTxHash)
	return tick, txHash
}

const (
	unnamedSystem = "unnamed_system"
)

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
//
//nolint:funlen // tick has a lot going on and doesn't really have a clear path to move things out.
func (e *Engine) Tick(ctx context.Context) error {
	nameOfCurrentRunningSystem := unnamedSystem
	defer func() {
		if panicValue := recover(); panicValue != nil {
			e.Logger.Error().
				Msgf("Tick: %d, Current running system: %s", e.CurrentTick(), nameOfCurrentRunningSystem)
			panic(panicValue)
		}
	}()
	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "cardinal.span.tick")
	defer func() {
		span.Finish()
	}()
	startTime := time.Now()
	e.Logger.Info().Int("tick", int(e.CurrentTick())).Msg("Tick started")
	if !e.stateIsLoaded {
		return eris.New("must load state before first tick")
	}
	txQueue := e.txQueue.CopyTransactions()

	if err := e.TickStore().StartNextTick(e.registeredMessages, txQueue); err != nil {
		return err
	}

	if e.CurrentTick() == 0 {
		eCtx := NewEngineContextForTick(e, txQueue, e.initSystemLogger)
		err := e.initSystem(eCtx)
		if err != nil {
			return err
		}
	}
	e.timestamp.Store(uint64(startTime.Unix()))
	allSystemStartTime := time.Now()
	for i, sys := range e.systems {
		nameOfCurrentRunningSystem = e.systemNames[i]
		eCtx := NewEngineContextForTick(e, txQueue, e.systemLoggers[i])
		systemStartTime := time.Now()
		err := eris.Wrapf(sys(eCtx), "system %s generated an error", nameOfCurrentRunningSystem)
		statsd.EmitTickStat(systemStartTime, nameOfCurrentRunningSystem)
		nameOfCurrentRunningSystem = unnamedSystem
		if err != nil {
			return err
		}
	}
	statsd.EmitTickStat(allSystemStartTime, "all_systems")
	if e.eventHub != nil {
		// engine can be optionally loaded with or without an eventHub. If there is one, on every tick it must flush events.
		flushEventStart := time.Now()
		e.eventHub.FlushEvents()
		statsd.EmitTickStat(flushEventStart, "flush_events")
	}

	finalizeTickStartTime := time.Now()
	if err := e.TickStore().FinalizeTick(ctx); err != nil {
		return err
	}
	statsd.EmitTickStat(finalizeTickStartTime, "finalize")

	e.setEvmResults(txQueue.GetEVMTxs())
	if txQueue.GetAmountOfTxs() != 0 && e.chain != nil && !e.isRecovering.Load() {
		err := e.chain.Submit(ctx, txQueue.Transactions(), e.namespace.String(), e.tick.Load(), e.timestamp.Load())
		if err != nil {
			return fmt.Errorf("failed to submit transactions to base shard: %w", err)
		}
	}

	e.tick.Add(1)
	e.receiptHistory.NextTick()
	statsd.EmitTickStat(startTime, "full_tick")
	if err := statsd.Client().Count("num_of_txs", int64(txQueue.GetAmountOfTxs()), nil, 1); err != nil {
		e.Logger.Warn().Msgf("failed to emit count stat:%v", err)
	}
	return nil
}

type EVMTxReceipt struct {
	ABIResult []byte
	Errs      []error
	EVMTxHash string
}

func (e *Engine) setEvmResults(txs []txpool.TxData) {
	// iterate over all EVM originated transactions
	for _, tx := range txs {
		// see if tx has a receipt. sometimes it won't because:
		// The system isn't using TxIterators && never explicitly called SetResult.
		rec, ok := e.receiptHistory.GetReceipt(tx.TxHash)
		if !ok {
			continue
		}
		evmRec := EVMTxReceipt{EVMTxHash: tx.EVMSourceTxHash}
		msg := e.getMessage(tx.MsgID)
		if rec.Result != nil {
			abiBz, err := msg.ABIEncode(rec.Result)
			if err != nil {
				rec.Errs = append(rec.Errs, err)
			}
			evmRec.ABIResult = abiBz
		}
		if len(rec.Errs) > 0 {
			evmRec.Errs = rec.Errs
		}
		e.evmTxReceipts[evmRec.EVMTxHash] = evmRec
	}
}

func (e *Engine) emitResourcesWarnings() {
	// todo: add links to docs related to each warning
	if !e.isComponentsRegistered {
		e.Logger.Warn().Msg("No components registered.")
	}
	if !e.isMessagesRegistered {
		e.Logger.Warn().Msg("No messages registered.")
	}
	if len(e.registeredQueries) == 0 {
		e.Logger.Warn().Msg("No queries registered.")
	}
	if len(e.systems) == 0 {
		e.Logger.Warn().Msg("No systems registered.")
	}
}

func (e *Engine) StartGameLoop(
	ctx context.Context,
	tickStart <-chan time.Time,
	tickDone chan<- uint64,
) {
	e.Logger.Info().Msg("Game loop started")
	ecslog.Engine(e.Logger, e, zerolog.InfoLevel)
	e.emitResourcesWarnings()

	go func() {
		ok := e.isGameLoopRunning.CompareAndSwap(false, true)
		if !ok {
			// The game has already started
			return
		}
		var waitingChs []chan struct{}
	loop:
		for {
			select {
			case <-tickStart:
				e.tickTheEngine(ctx, tickDone)
				closeAllChannels(waitingChs)
				waitingChs = waitingChs[:0]
			case <-e.endGameLoopCh:
				e.drainChannelsWaitingForNextTick()
				e.drainEndLoopChannels()
				closeAllChannels(waitingChs)
				if e.GetTxQueueAmount() > 0 {
					// immediately tick if queue is not empty to process all txs if queue is not empty.
					e.tickTheEngine(ctx, tickDone)
					if tickDone != nil {
						close(tickDone)
					}
				}
				break loop
			case ch := <-e.addChannelWaitingForNextTick:
				waitingChs = append(waitingChs, ch)
			}
		}
		e.isGameLoopRunning.Store(false)
	}()
}

func closeAllChannels(chs []chan struct{}) {
	for _, ch := range chs {
		close(ch)
	}
}

func (e *Engine) tickTheEngine(ctx context.Context, tickDone chan<- uint64) {
	currTick := e.CurrentTick()
	// this is the final point where errors bubble up and hit a panic. There are other places where this occurs
	// but this is the highest terminal point.
	// the panic may point you to here, (or the tick function) but the real stack trace is in the error message.
	if err := e.Tick(ctx); err != nil {
		bytes, err := json.Marshal(eris.ToJSON(err, true))
		if err != nil {
			panic(err)
		}
		e.Logger.Panic().Err(err).Str("tickError", "Error running Tick in Game Loop.").RawJSON("error", bytes)
	}
	if tickDone != nil {
		tickDone <- currTick
	}
}

// drainChannelsWaitingForNextTick continually closes any channels that are added to the
// addChannelWaitingForNextTick channel. This is used when the engine is shut down; it ensures
// any calls to WaitForNextTick that happen after a shutdown will not block.
func (e *Engine) drainChannelsWaitingForNextTick() {
	go func() {
		for ch := range e.addChannelWaitingForNextTick {
			close(ch)
		}
	}()
}

func (e *Engine) drainEndLoopChannels() {
	go func() {
		for range e.endGameLoopCh { //nolint:revive // This pattern drains the channel until closed
		}
	}()
}

// WaitForNextTick blocks until at least one game tick has completed. It returns true if it successfully waited for a
// tick. False may be returned if the engine was shut down while waiting for the next tick to complete.
func (e *Engine) WaitForNextTick() (success bool) {
	startTick := e.CurrentTick()
	ch := make(chan struct{})
	e.addChannelWaitingForNextTick <- ch
	<-ch
	return e.CurrentTick() > startTick
}

func (e *Engine) IsGameLoopRunning() bool {
	return e.isGameLoopRunning.Load()
}

func (e *Engine) Shutdown() {
	e.shutdownMutex.Lock() // This queues up Shutdown calls so they happen one after the other.
	defer e.shutdownMutex.Unlock()
	if !e.IsGameLoopRunning() {
		return
	}
	log.Info().Msg("Shutting down game loop.")
	e.endGameLoopCh <- true
	for e.IsGameLoopRunning() { // Block until loop stops.
		time.Sleep(100 * time.Millisecond) //nolint:gomnd // its ok.
	}
	log.Info().Msg("Successfully shut down game loop.")
	if e.eventHub != nil {
		e.eventHub.ShutdownEventHub()
	}
}

// recoverGameState checks the status of the last game tick. If the tick was incomplete (indicating
// a problem when running one of the Systems), the snapshotted state is recovered and the pending
// transactions for the incomplete tick are returned. A nil recoveredTxs indicates there are no pending
// transactions that need to be processed because the last tick was successful.
func (e *Engine) recoverGameState() (recoveredTxs *txpool.TxQueue, err error) {
	start, end, err := e.TickStore().GetTickNumbers()
	if err != nil {
		return nil, err
	}
	e.tick.Store(end)
	// We successfully completed the last tick. Everything is fine
	if start == end {
		//nolint:nilnil // its ok.
		return nil, nil
	}
	return e.TickStore().Recover(e.registeredMessages)
}

func (e *Engine) LoadGameState() error {
	if e.IsEntitiesCreated() {
		return eris.Wrap(ErrEntitiesCreatedBeforeLoadingGameState, "")
	}
	if e.stateIsLoaded {
		return eris.New("cannot load game state multiple times")
	}
	if !e.isMessagesRegistered {
		if err := e.RegisterMessages(); err != nil {
			return err
		}
	}

	if !e.isComponentsRegistered {
		err := RegisterComponent[SignerComponent](e)
		if err != nil {
			return err
		}
	}

	if err := e.entityStore.RegisterComponents(e.registeredComponents); err != nil {
		return err
	}

	e.stateIsLoaded = true
	recoveredTxs, err := e.recoverGameState()
	if err != nil {
		return err
	}

	if recoveredTxs != nil {
		e.txQueue = recoveredTxs
		if err = e.Tick(context.Background()); err != nil {
			return err
		}
	}
	e.receiptHistory.SetTick(e.CurrentTick())

	return nil
}

// RecoverFromChain will attempt to recover the state of the engine based on historical transaction data.
// The function puts the engine in a recovery state, and then queries all transaction batches under the engine's
// namespace. The function will continuously ask the EVM base shard for batches, and run ticks for each batch returned.
//
//nolint:gocognit
func (e *Engine) RecoverFromChain(ctx context.Context) error {
	if e.chain == nil {
		return eris.Errorf(
			"chain adapter was nil. " +
				"be sure to use the `WithAdapter` option when creating the world",
		)
	}
	if e.CurrentTick() > 0 {
		return eris.Errorf(
			"world recovery should not occur in a world with existing state. please verify all " +
				"state has been cleared before running recovery",
		)
	}

	e.isRecovering.Store(true)
	defer func() {
		e.isRecovering.Store(false)
	}()
	namespace := e.Namespace().String()
	var nextKey []byte
	for {
		res, err := e.chain.QueryTransactions(
			ctx, &types.QueryTransactionsRequest{
				Namespace: namespace,
				Page: &types.PageRequest{
					Key: nextKey,
				},
			},
		)
		if err != nil {
			return err
		}
		for _, tickedTxs := range res.Epochs {
			target := tickedTxs.Epoch
			// tick up to target
			if target < e.CurrentTick() {
				return eris.Errorf(
					"got tx for tick %d, but world is at tick %d",
					target,
					e.CurrentTick(),
				)
			}
			for current := e.CurrentTick(); current != target; {
				if err = e.Tick(ctx); err != nil {
					return err
				}
				current = e.CurrentTick()
			}
			// we've now reached target. we need to inject the transactions and tick.
			transactions := tickedTxs.Txs
			for _, tx := range transactions {
				sp, err := e.decodeTransaction(tx.GameShardTransaction)
				if err != nil {
					return err
				}
				msg := e.getMessage(message.TypeID(tx.TxId))
				if msg == nil {
					return eris.Errorf("error recovering tx with ID %d: tx id not found", tx.TxId)
				}
				v, err := msg.Decode(sp.Body)
				if err != nil {
					return err
				}
				e.AddTransaction(message.TypeID(tx.TxId), v, e.protoTransactionToGo(sp))
			}
			// run the tick for this batch
			if err = e.Tick(ctx); err != nil {
				return err
			}
		}

		// if a page response was in the reply, that means there is more data to read.
		if res.Page != nil {
			// case where the next key is empty or nil, we don't want to continue the queries.
			if res.Page.Key == nil || len(res.Page.Key) == 0 {
				break
			}
			nextKey = res.Page.Key
		} else {
			// if the entire page reply is nil, then we are definitely done.
			break
		}
	}
	return nil
}

func (e *Engine) protoTransactionToGo(sp *shardv1.Transaction) *sign.Transaction {
	return &sign.Transaction{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}

func (e *Engine) decodeTransaction(bz []byte) (*shardv1.Transaction, error) {
	payload := new(shardv1.Transaction)
	err := proto.Unmarshal(bz, payload)
	return payload, eris.Wrap(err, "")
}

// getMessage iterates over the all registered messages and returns the message.Message associated with the
// message.TypeID.
func (e *Engine) getMessage(id message.TypeID) message.Message {
	for _, msg := range e.registeredMessages {
		if id == msg.ID() {
			return msg
		}
	}
	return nil
}

func (e *Engine) UseNonce(signerAddress string, nonce uint64) error {
	return e.redisStorage.UseNonce(signerAddress, nonce)
}

func (e *Engine) AddMessageError(id message.TxHash, err error) {
	e.receiptHistory.AddError(id, err)
}

func (e *Engine) SetMessageResult(id message.TxHash, a any) {
	e.receiptHistory.SetResult(id, a)
}

func (e *Engine) GetTransactionReceipt(id message.TxHash) (any, []error, bool) {
	rec, ok := e.receiptHistory.GetReceipt(id)
	if !ok {
		return nil, nil, false
	}
	return rec.Result, rec.Errs, true
}

func (e *Engine) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return e.receiptHistory.GetReceiptsForTick(tick)
}

func (e *Engine) GetComponents() []component.ComponentMetadata {
	return e.registeredComponents
}

func (e *Engine) GetSystemNames() []string {
	return e.systemNames
}

func (e *Engine) InjectLogger(logger *zerolog.Logger) {
	e.Logger = logger
	e.GameStateManager().InjectLogger(logger)
}

func (e *Engine) NewSearch(filter filter.ComponentFilter) *Search {
	return NewSearch(filter)
}
