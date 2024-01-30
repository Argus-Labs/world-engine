package ecs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"pkg.world.dev/world-engine/cardinal/ecs/messages"
	"pkg.world.dev/world-engine/cardinal/ecs/systems"
	"pkg.world.dev/world-engine/cardinal/types/engine"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/ecs/gamestate"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/storage/redis"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/statsd"
	"pkg.world.dev/world-engine/cardinal/txpool"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/message"
	"pkg.world.dev/world-engine/sign"
)

// EngineStateType is the current state of the engine.
type EngineStateType string

const (
	defaultReceiptHistorySize = 10
)

const (
	EngineStateInit       EngineStateType = "EngineStateInit"
	EngineStateRecovering EngineStateType = "EngineStateRecovering"
	EngineStateReady      EngineStateType = "EngineStateReady"
	EngineStateRunning    EngineStateType = "EngineStateRunning"
)

var (
	ErrEntitiesCreatedBeforeLoadingGameState = errors.New("cannot create entities before loading game state")
	ErrStoreStateInvalid                     = errors.New("saved engine state is not valid")
	ErrDuplicateQueryName                    = errors.New("query names must be unique")
)

type Engine struct {
	EngineState            EngineStateType
	namespace              Namespace
	redisStorage           *redis.Storage
	entityStore            gamestate.Manager
	tick                   *atomic.Uint64
	timestamp              *atomic.Uint64
	nameToComponent        map[string]component.ComponentMetadata
	nameToQuery            map[string]Query
	registeredComponents   []component.ComponentMetadata
	registeredQueries      []Query
	isComponentsRegistered bool

	// New managers from refactor
	msgManager    *msgs.Manager
	systemManager *systems.Manager

	evmTxReceipts map[string]EVMTxReceipt

	txQueue *txpool.TxQueue

	receiptHistory *receipt.History

	router router.Router
	// isRecovering indicates that the engine is recovering from the DA layer.
	// this is used to prevent ticks from submitting duplicate transactions the DA layer.
	isRecovering atomic.Bool

	Logger *zerolog.Logger

	endGameLoopCh     chan bool
	isGameLoopRunning atomic.Bool

	nextComponentID component.TypeID

	eventHub *events.EventHub

	// addChannelWaitingForNextTick accepts a channel which will be closed after a tick has been completed.
	addChannelWaitingForNextTick chan chan struct{}

	shutdownMutex sync.Mutex
}

func (e *Engine) ConsumeEVMMsgResult(evmTxHash string) ([]byte, []error, string, bool) {
	rcpt, exists := e.consumeEVMMsgResult(evmTxHash)
	return rcpt.ABIResult, rcpt.Errs, rcpt.EVMTxHash, exists
}

func (e *Engine) HandleEVMQuery(name string, abiRequest []byte) ([]byte, error) {
	qry, err := e.GetQueryByName(name)
	if err != nil {
		return nil, err
	}
	req, err := qry.DecodeEVMRequest(abiRequest)
	if err != nil {
		return nil, err
	}

	reply, err := qry.HandleQuery(NewReadOnlyEngineContext(e), req)
	if err != nil {
		return nil, err
	}

	return qry.EncodeEVMReply(reply)
}

func (e *Engine) GetEVMMsgResult(evmTxHash string) (EVMTxReceipt, bool) {
	return e.consumeEVMMsgResult(evmTxHash)
}

func (e *Engine) GetMessageByName(s string) (message.Message, bool) {
	for _, msg := range e.registeredMessages {
		if msg.Name() == s {
			return msg, true
		}
	}
	return nil, false
}

func (e *Engine) GetPersonaForEVMAddress(addr string) (string, error) {
	var sc *SignerComponent
	eCtx := NewReadOnlyEngineContext(e)
	q := eCtx.NewSearch(filter.Exact(SignerComponent{}))
	var getComponentErr error
	searchIterationErr := eris.Wrap(
		q.Each(
			eCtx, func(id entity.ID) bool {
				var signerComp *SignerComponent
				signerComp, getComponentErr = GetComponent[SignerComponent](eCtx, id)
				getComponentErr = eris.Wrap(getComponentErr, "")
				if getComponentErr != nil {
					return false
				}
				for _, authAddr := range signerComp.AuthorizedAddresses {
					if authAddr == addr {
						sc = signerComp
						return false
					}
				}
				return true
			},
		), "",
	)
	if getComponentErr != nil {
		return "", getComponentErr
	}
	if searchIterationErr != nil {
		return "", searchIterationErr
	}
	if sc == nil {
		return "", eris.Errorf("address %s does not have a linked persona tag", addr)
	}
	return sc.PersonaTag, nil
}

var (
	ErrEntitiesCreatedBeforeLoadingGameState = errors.New("cannot create entities before loading game state")
	ErrStoreStateInvalid                     = errors.New("saved engine state is not valid")
	ErrDuplicateQueryName                    = errors.New("query names must be unique")
)

		msgManager:    msgs.New(),
		systemManager: systems.New(),
		EngineState:   EngineStateInit,

		addChannelWaitingForNextTick: make(chan chan struct{}),
	}
	// TODO(scott): move this into an internal plugin
	e.registerInternalQueries()

	e.isGameLoopRunning.Store(false)
	for _, opt := range opts {
		opt(e)
	}
	if e.receiptHistory == nil {
		e.receiptHistory = receipt.NewHistory(e.CurrentTick(), defaultReceiptHistorySize)
	}
	if e.eventHub == nil {
		e.eventHub = events.NewEventHub()
	}

	return e, nil
}

func (e *Engine) GetEventHub() *events.EventHub {
	return e.eventHub
}

func (e *Engine) EmitEvent(event *events.Event) {
	e.eventHub.EmitEvent(event)
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

func RegisterQuery[Request any, Reply any](
	engine *Engine,
	name string,
	handler func(eCtx engine.Context, req *Request) (*Reply, error),
	opts ...QueryOption[Request, Reply],
) error {
	if engine.EngineState != EngineStateInit {
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

func (e *Engine) registerInternalQueries() {
	debugQueryType, err := NewQueryType[DebugRequest, DebugStateResponse](
		"state",
		queryDebugState,
		WithCustomQueryGroup[DebugRequest, DebugStateResponse]("debug"),
	)
	if err != nil {
		panic(err)
	}

	cqlQueryType, err := NewQueryType[CQLQueryRequest, CQLQueryResponse]("cql", queryCQL)
	if err != nil {
		panic(err)
	}

	receiptQueryType, err := NewQueryType[ListTxReceiptsRequest, ListTxReceiptsReply](
		"list",
		receiptsQuery,
		WithCustomQueryGroup[ListTxReceiptsRequest, ListTxReceiptsReply]("receipts"),
	)
	if err != nil {
		panic(err)
	}
	e.registeredQueries = append(
		e.registeredQueries,
		debugQueryType,
		cqlQueryType,
		receiptQueryType,
	)
}

func (e *Engine) ListQueries() []Query {
	return e.registeredQueries
}

func (e *Engine) ListMessages() []message.Message { return e.msgManager.GetRegisteredMessages() }

func (e *Engine) CurrentTick() uint64 {
	return e.tick.Load()
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

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (e *Engine) Tick(ctx context.Context) error {
	// If the engine is not ready, we don't want to tick the engine.
	// Instead, we want to make sure we have recovered the state of the engine.
	if e.EngineState != EngineStateReady && e.EngineState != EngineStateRunning {
		return eris.New("must load state before first tick")
	}
	e.EngineState = EngineStateRunning

	// This defer is here to catch any panics that occur during the tick. It will log the current tick and the
	// current system that is running.
	defer func() {
		if panicValue := recover(); panicValue != nil {
			e.Logger.Error().
				Msgf("Tick: %d, Current running system: %s", e.CurrentTick(), e.systemManager.GetCurrentSystem())
			panic(panicValue)
		}
	}()

	var span tracer.Span
	span, ctx = tracer.StartSpanFromContext(ctx, "cardinal.span.tick")
	defer func() {
		span.Finish()
	}()

	e.Logger.Info().Int("tick", int(e.CurrentTick())).Msg("Tick started")

	// Copy the transactions from the queue so that we can safely modify the queue while the tick is running.
	txQueue := e.txQueue.CopyTransactions()

	if err := e.TickStore().StartNextTick(e.msgManager.GetRegisteredMessages(), txQueue); err != nil {
		return err
	}

	// Set the timestamp for this tick
	startTime := time.Now()
	e.timestamp.Store(uint64(startTime.Unix()))

	// Create the engine context to inject into systems
	eCtx := NewEngineContextForTick(e, txQueue, e.Logger)

	// Run the init system on the first tick
	if e.CurrentTick() == 0 {
		err := e.systemManager.RunInitSystem(eCtx)
		if err != nil {
			return err
		}
	}

	// Run all the systems
	err := e.systemManager.RunSystems(eCtx)
	if err != nil {
		return err
	}

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
	if txQueue.GetAmountOfTxs() != 0 && e.router != nil && !e.isRecovering.Load() {
		err := e.router.SubmitTxBlob(ctx, txQueue.Transactions(), e.namespace.String(), e.tick.Load(), e.timestamp.Load())
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

func (e *Engine) emitResourcesWarnings() {
	// todo: add links to docs related to each warning
	if !e.isComponentsRegistered {
		e.Logger.Warn().Msg("No components registered.")
	}
	if !e.msgManager.IsMessagesRegistered() {
		e.Logger.Warn().Msg("No messages registered.")
	}
	if len(e.registeredQueries) == 0 {
		e.Logger.Warn().Msg("No queries registered.")
	}
	if !e.systemManager.IsSystemsRegistered() {
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

func (e *Engine) Shutdown() error {
	e.shutdownMutex.Lock() // This queues up Shutdown calls so they happen one after the other.
	defer e.shutdownMutex.Unlock()
	if !e.IsGameLoopRunning() {
		return nil
	}
	log.Info().Msg("Shutting down game loop.")
	e.endGameLoopCh <- true
	for e.IsGameLoopRunning() { // Block until loop stops.
		time.Sleep(100 * time.Millisecond) //nolint:gomnd // its ok.
	}
	log.Info().Msg("Successfully shut down game loop.")
	if e.eventHub != nil {
		e.eventHub.Shutdown()
	}
	log.Info().Msg("Closing storage connection.")
	err := e.redisStorage.Close()
	if err != nil {
		log.Error().Err(err).Msg("Failed to close storage connection.")
		return err
	}
	log.Info().Msg("Successfully closed storage connection.")
	if e.router != nil {
		e.router.Shutdown()
	}
	return nil
}

func (e *Engine) LoadGameState() error {
	if e.EngineState != EngineStateInit {
		return eris.New("cannot load game state multiple times")
	}

	// TODO(scott): footgun. so confusing.
	if err := e.entityStore.RegisterComponents(e.registeredComponents); err != nil {
		e.entityStore.Close()
		return err
	}

	recoveredTxs, err := e.recoverGameState()
	if err != nil {
		return err
	}

	// Engine is now ready to run
	e.EngineState = EngineStateReady

	// Recover the last tick, not to be confused with RecoverFromChain
	// It's ambigious, but screw it for now.
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
	if e.router == nil {
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
		res, err := e.router.QueryTransactions(
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
				msg := e.msgManager.GetMessage(message.TypeID(tx.TxId))
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

func (e *Engine) UseNonce(signerAddress string, nonce uint64) error {
	return e.redisStorage.UseNonce(signerAddress, nonce)
}

func (e *Engine) InjectLogger(logger *zerolog.Logger) {
	e.Logger = logger
	e.GameStateManager().InjectLogger(logger)
}
