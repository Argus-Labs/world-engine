package ecs

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	shardv1 "buf.build/gen/go/argus-labs/world-engine/protocolbuffers/go/shard/v1"
	"google.golang.org/protobuf/proto"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/archetype"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/ecs/transaction"
	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/chain/x/shard/types"
	"pkg.world.dev/world-engine/sign"
)

// Namespace is a unique identifier for a world.
type Namespace string

type World struct {
	namespace                Namespace
	store                    storage.WorldStorage
	storeManager             store.IManager
	systems                  []System
	systemLoggers            []*ecslog.Logger
	systemNames              []string
	tick                     uint64
	nameToComponent          map[string]IComponentType
	registeredComponents     []IComponentType
	registeredTransactions   []transaction.ITransaction
	registeredQueries        []IQuery
	isComponentsRegistered   bool
	isTransactionsRegistered bool
	stateIsLoaded            bool

	txQueue *transaction.TxQueue

	receiptHistory *receipt.History

	chain shard.QueryAdapter
	// isRecovering indicates that the world is recovering from the DA layer.
	// this is used to prevent ticks from submitting duplicate transactions the DA layer.
	isRecovering bool

	Logger *ecslog.Logger

	endGameLoopCh     chan bool
	isGameLoopRunning atomic.Bool

	nextComponentID component.TypeID
}

var (
	ErrorTransactionRegistrationMustHappenOnce = errors.New("transaction registration must happen exactly 1 time")
	ErrorStoreStateInvalid                     = errors.New("saved world state is not valid")
	ErrorDuplicateTransactionName              = errors.New("transaction names must be unique")
	ErrorDuplicateQueryName                    = errors.New("query names must be unique")
)

func (w *World) IsRecovering() bool {
	return w.isRecovering
}

func (w *World) StoreManager() store.IManager {
	return w.storeManager
}

func (w *World) TickStore() storage.TickStorage {
	if ts, ok := w.StoreManager().(storage.TickStorage); ok {
		return ts
	}
	return w.store.TickStore
}

func (w *World) GetTxQueueAmount() int {
	return w.txQueue.GetAmountOfTxs()
}

func (w *World) AddSystem(s System) {
	w.AddSystems(s)
}

func (w *World) AddSystems(s ...System) {
	if w.stateIsLoaded {
		panic("cannot register systems after loading game state")
	}
	for _, system := range s {
		// retrieves function name from system using a reflection trick
		functionName := filepath.Base(runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name())
		sysLogger := w.Logger.CreateSystemLogger(functionName)
		w.systemLoggers = append(w.systemLoggers, &sysLogger)
		w.systemNames = append(w.systemNames, functionName)
		// appends registeredSystem into the member system list in world.
		w.systems = append(w.systems, system)
	}
}

func RegisterComponent[T component.IAbstractComponent](world *World) error {
	if world.stateIsLoaded {
		panic("cannot register components after loading game state")
	}
	var t T
	_, err := world.GetComponentByName(t.Name())
	if err == nil {
		return fmt.Errorf("component with name '%s' is already registered", t.Name())
	}
	c := NewComponentType[T]()
	err = c.SetID(world.nextComponentID)
	if err != nil {
		return err
	}
	world.registeredComponents = append(world.registeredComponents, c)
	world.nextComponentID += 1
	world.nameToComponent[t.Name()] = c
	world.isComponentsRegistered = true
	return nil
}

func (w *World) GetComponentByName(name string) (IComponentType, error) {
	componentType, exists := w.nameToComponent[name]
	if !exists {
		return nil, fmt.Errorf("Component with name %s not found. Must register component before using", name)
	}
	return componentType, nil
}

func (w *World) RegisterQueries(queries ...IQuery) error {
	if w.stateIsLoaded {
		panic("cannot register queries after loading game state")
	}
	w.registeredQueries = append(w.registeredQueries, queries...)
	seenQueryNames := map[string]struct{}{}
	for _, t := range w.registeredQueries {
		name := t.Name()
		if _, ok := seenQueryNames[name]; ok {
			return fmt.Errorf("duplicate query %q: %w", name, ErrorDuplicateQueryName)
		}
		seenQueryNames[name] = struct{}{}
	}
	return nil
}

func (w *World) RegisterTransactions(txs ...transaction.ITransaction) error {
	if w.stateIsLoaded {
		panic("cannot register transactions after loading game state")
	}
	if w.isTransactionsRegistered {
		return ErrorTransactionRegistrationMustHappenOnce
	}
	w.isTransactionsRegistered = true
	w.registerInternalTransactions()
	w.registeredTransactions = append(w.registeredTransactions, txs...)

	seenTxNames := map[string]bool{}
	for i, t := range w.registeredTransactions {
		name := t.Name()
		if seenTxNames[name] {
			return fmt.Errorf("duplicate tx %q: %w", name, ErrorDuplicateTransactionName)
		}
		seenTxNames[name] = true

		id := transaction.TypeID(i + 1)
		if err := t.SetID(id); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) registerInternalTransactions() {
	w.registeredTransactions = append(w.registeredTransactions,
		CreatePersonaTx,
		AuthorizePersonaAddressTx,
	)
}

func (w *World) ListQueries() []IQuery {
	return w.registeredQueries
}

func (w *World) ListTransactions() ([]transaction.ITransaction, error) {
	if !w.isTransactionsRegistered {
		return nil, errors.New("cannot list transactions until transaction registration occurs")
	}
	return w.registeredTransactions, nil
}

// NewWorld creates a new world.
func NewWorld(s storage.WorldStorage, sm store.IManager, opts ...Option) (*World, error) {
	logger := &ecslog.Logger{
		&log.Logger,
	}
	sm.InjectLogger(logger)
	w := &World{
		store:             s,
		storeManager:      sm,
		namespace:         "world",
		tick:              0,
		systems:           make([]System, 0),
		nameToComponent:   make(map[string]IComponentType),
		txQueue:           transaction.NewTxQueue(),
		Logger:            logger,
		isGameLoopRunning: atomic.Bool{},
		endGameLoopCh:     make(chan bool),
		nextComponentID:   1,
	}
	w.isGameLoopRunning.Store(false)
	w.AddSystems(RegisterPersonaSystem, AuthorizePersonaAddressSystem)
	err := RegisterComponent[SignerComponent](w)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(w)
	}
	if w.receiptHistory == nil {
		w.receiptHistory = receipt.NewHistory(w.CurrentTick(), 10)
	}
	return w, nil
}

func (w *World) CurrentTick() uint64 {
	return w.tick
}

func (w *World) ReceiptHistorySize() uint64 {
	return w.receiptHistory.Size()
}

// Len return the number of entities in this world
func (w *World) Len() (int, error) {
	l, err := w.store.EntityLocStore.Len()
	if err != nil {
		return 0, err
	}
	return l, nil
}

// Remove removes the given Entity from the world
func (w *World) Remove(id entity.ID) error {
	return w.StoreManager().RemoveEntity(id)
}

// AddTransaction adds a transaction to the transaction queue. This should not be used directly.
// Instead, use a TransactionType.AddToQueue to ensure type consistency. Returns the tick this transaction will be
// executed in.
func (w *World) AddTransaction(id transaction.TypeID, v any, sig *sign.SignedPayload) (tick uint64, txHash transaction.TxHash) {
	// TODO: There's no locking between getting the tick and adding the transaction, so there's no guarantee that this
	// transaction is actually added to the returned tick.
	tick = w.CurrentTick()
	txHash = w.txQueue.AddTransaction(id, v, sig)
	return tick, txHash
}

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (w *World) Tick(ctx context.Context) error {
	nullSystemName := "No system  is running."
	nameOfCurrentRunningSystem := nullSystemName
	defer func() {
		if panicValue := recover(); panicValue != nil {
			w.Logger.Error().Msgf("Tick: %d, Current running system: %s", w.tick, nameOfCurrentRunningSystem)
			panic(panicValue)
		}
	}()
	startTime := time.Now()
	warningThreshold := 100 * time.Millisecond
	tickAsString := strconv.FormatUint(w.tick, 10)
	w.Logger.Info().Str("tick", tickAsString).Msg("Tick started")
	if !w.stateIsLoaded {
		return errors.New("must load state before first tick")
	}
	txQueue := w.txQueue.CopyTransaction()

	if err := w.TickStore().StartNextTick(w.registeredTransactions, txQueue); err != nil {
		return err
	}

	for i, sys := range w.systems {
		nameOfCurrentRunningSystem = w.systemNames[i]
		err := sys(w, txQueue, w.systemLoggers[i])
		nameOfCurrentRunningSystem = nullSystemName
		if err != nil {
			return err
		}
	}

	if err := w.TickStore().FinalizeTick(); err != nil {
		return err
	}

	w.tick++
	w.receiptHistory.NextTick()
	elapsedTime := time.Since(startTime)

	var logEvent *zerolog.Event
	message := "tick ended"
	if elapsedTime > warningThreshold {
		logEvent = w.Logger.Warn()
		message = message + fmt.Sprintf(", (warning: tick exceeded %dms)", warningThreshold.Milliseconds())
	} else {
		logEvent = w.Logger.Info()
	}
	logEvent.
		Int("tick_execution_time", int(elapsedTime.Milliseconds())).
		Str("tick", tickAsString).
		Msg(message)
	return nil
}

func (w *World) StartGameLoop(ctx context.Context, tickStart <-chan time.Time, tickDone chan<- uint64) {
	w.Logger.Info().Msg("Game loop started")
	w.Logger.LogWorld(w, zerolog.InfoLevel)
	//todo: add links to docs related to each warning
	if !w.isComponentsRegistered {
		w.Logger.Warn().Msg("No components registered.")
	}
	if !w.isTransactionsRegistered {
		w.Logger.Warn().Msg("No transactions registered.")
	}
	if len(w.registeredQueries) == 0 {
		w.Logger.Warn().Msg("No queries registered.")
	}
	if len(w.systems) == 0 {
		w.Logger.Warn().Msg("No systems registered.")
	}

	go func() {
		tickTheWorld := func() {
			currTick := w.CurrentTick()
			if err := w.Tick(ctx); err != nil {
				w.Logger.Panic().Err(err).Msg("Error running Tick in Game Loop.")
			}
			if tickDone != nil {
				tickDone <- currTick
			}
		}
		w.isGameLoopRunning.Store(true)
	loop:
		for {
			select {
			case <-tickStart:
				tickTheWorld()
			case <-w.endGameLoopCh:
				if w.GetTxQueueAmount() > 0 {
					tickTheWorld() //immediately tick if queue is not empty to process all txs if queue is not empty.
				}
				break loop
			}
		}
		w.isGameLoopRunning.Store(false)
	}()
}

func (w *World) IsGameLoopRunning() bool {
	return w.isGameLoopRunning.Load()
}

func (w *World) EndGameLoop() {
	if !w.IsGameLoopRunning() {
		return
	}
	w.endGameLoopCh <- true
	for w.IsGameLoopRunning() { //Block until loop stops.
		time.Sleep(100 * time.Millisecond)
	}
}

type TxBatch struct {
	TxID transaction.TypeID `json:"tx_id,omitempty"`
	Txs  []any              `json:"txs,omitempty"`
}

const (
	storeArchetypeCompIdxKey  = "arch_component_index"
	storeArchetypeAccessorKey = "arch_accessor"
)

// recoverGameState checks the status of the last game tick. If the tick was incomplete (indicating
// a problem when running one of the Systems), the snapshotted state is recovered and the pending
// transactions for the incomplete tick are returned. A nil recoveredTxs indicates there are no pending
// transactions that need to be processed because the last tick was successful.
func (w *World) recoverGameState() (recoveredTxs *transaction.TxQueue, err error) {
	start, end, err := w.TickStore().GetTickNumbers()
	if err != nil {
		return nil, err
	}
	w.tick = end
	// We successfully completed the last tick. Everything is fine
	if start == end {
		return nil, nil
	}
	return w.TickStore().Recover(w.registeredTransactions)
}

func (w *World) LoadGameState() error {
	if w.stateIsLoaded {
		return errors.New("cannot load game state multiple times")
	}
	if !w.isTransactionsRegistered {
		if err := w.RegisterTransactions(); err != nil {
			return err
		}
	}

	if !w.isComponentsRegistered {
		err := RegisterComponent[SignerComponent](w)
		if err != nil {
			return err
		}
	}

	if err := w.storeManager.RegisterComponents(w.registeredComponents); err != nil {
		return err
	}

	w.stateIsLoaded = true
	recoveredTxs, err := w.recoverGameState()
	if err != nil {
		return err
	}

	if err := w.loadFromKey(storeArchetypeAccessorKey, w.store.ArchAccessor, w.registeredComponents); err != nil {
		return err
	}
	if err := w.loadFromKey(storeArchetypeCompIdxKey, w.store.ArchCompIdxStore, w.registeredComponents); err != nil {
		return err
	}

	if recoveredTxs != nil {
		w.txQueue = recoveredTxs
		if err := w.Tick(context.Background()); err != nil {
			return err
		}
	}
	w.receiptHistory.SetTick(w.tick)

	return nil
}

func (w *World) loadFromKey(key string, cm storage.ComponentMarshaler, comps []IComponentType) error {
	buf, ok, err := w.store.StateStore.Load(key)
	if !ok {
		// There is no saved data for this key
		return nil
	} else if err != nil {
		return err
	}
	return cm.UnmarshalWithComps(buf, comps)
}

func (w *World) nextEntity() (entity.ID, error) {
	return w.store.EntityMgr.NewEntity()
}

func (w *World) insertArchetype(comps []IComponentType) archetype.ID {
	w.store.ArchCompIdxStore.Push(comps)
	archID := archetype.ID(w.store.ArchAccessor.Count())

	w.store.ArchAccessor.PushArchetype(archID, comps)
	w.Logger.Debug().Int("archetype_id", int(archID)).Msg("created")
	return archID
}

// RecoverFromChain will attempt to recover the state of the world based on historical transaction data.
// The function puts the world in a recovery state, and then queries all transaction batches under the world's
// namespace. The function will continuously ask the EVM base shard for batches, and run ticks for each batch returned.
func (w *World) RecoverFromChain(ctx context.Context) error {
	if w.chain == nil {
		return fmt.Errorf("chain adapter was nil. " +
			"be sure to use the `WithAdapter` option when creating the world")
	}
	if w.tick > 0 {
		return fmt.Errorf("world recovery should not occur in a world with existing state. please verify all " +
			"state has been cleared before running recovery")
	}

	w.isRecovering = true
	defer func() {
		w.isRecovering = false
	}()
	namespace := w.Namespace()
	var nextKey []byte
	for {
		res, err := w.chain.QueryTransactions(ctx, &types.QueryTransactionsRequest{
			Namespace: namespace,
			Page: &types.PageRequest{
				Key: nextKey,
			},
		})
		if err != nil {
			return err
		}
		for _, tickedTxs := range res.Epochs {
			target := tickedTxs.Epoch
			// tick up to target
			if target < w.CurrentTick() {
				return fmt.Errorf("got tx for tick %d, but world is at tick %d", target, w.CurrentTick())
			}
			for current := w.CurrentTick(); current != target; {
				if err := w.Tick(ctx); err != nil {
					return err
				}
				current = w.CurrentTick()
			}
			// we've now reached target. we need to inject the transactions and tick.
			transactions := tickedTxs.Txs
			for _, tx := range transactions {
				sp, err := w.decodeTransaction(tx.SignedPayload)
				if err != nil {
					return err
				}
				itx := w.getITx(transaction.TypeID(tx.TxId))
				if itx == nil {
					return fmt.Errorf("error recovering tx with ID %d: tx id not found", tx.TxId)
				}
				v, err := itx.Decode(sp.Body)
				if err != nil {
					return err
				}
				w.AddTransaction(transaction.TypeID(tx.TxId), v, w.protoSignedPayloadToGo(sp))
			}
			// run the tick for this batch
			if err := w.Tick(ctx); err != nil {
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

func (w *World) protoSignedPayloadToGo(sp *shardv1.SignedPayload) *sign.SignedPayload {
	return &sign.SignedPayload{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}

func (w *World) decodeTransaction(bz []byte) (*shardv1.SignedPayload, error) {
	payload := new(shardv1.SignedPayload)
	err := proto.Unmarshal(bz, payload)
	return payload, err
}

// getITx iterates over the registered transactions and returns the ITransaction associated with the TypeID.
func (w *World) getITx(id transaction.TypeID) transaction.ITransaction {
	var itx transaction.ITransaction
	for _, tx := range w.registeredTransactions {
		if id == tx.ID() {
			itx = tx
			break
		}
	}
	return itx
}

// Namespace returns the world's namespace.
func (w *World) Namespace() string {
	return string(w.namespace)
}

func (w *World) GetNonce(signerAddress string) (uint64, error) {
	return w.store.NonceStore.GetNonce(signerAddress)
}

func (w *World) SetNonce(signerAddress string, nonce uint64) error {
	return w.store.NonceStore.SetNonce(signerAddress, nonce)
}

func (w *World) AddTransactionError(id transaction.TxHash, err error) {
	w.receiptHistory.AddError(id, err)
}

func (w *World) SetTransactionResult(id transaction.TxHash, a any) {
	w.receiptHistory.SetResult(id, a)
}

func (w *World) GetTransactionReceipt(id transaction.TxHash) (any, []error, bool) {
	rec, ok := w.receiptHistory.GetReceipt(id)
	if !ok {
		return nil, nil, false
	}
	return rec.Result, rec.Errs, true
}

func (w *World) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return w.receiptHistory.GetReceiptsForTick(tick)
}

func (w *World) GetComponents() []IComponentType {
	return w.registeredComponents
}

func (w *World) GetSystemNames() []string {
	return w.systemNames
}

func (w *World) InjectLogger(logger *ecslog.Logger) {
	w.Logger = logger
	w.StoreManager().InjectLogger(logger)
}

func (w *World) NewQuery(filter Filterable) (*Query, error) {
	componentFilter, err := filter.ConvertToComponentFilter(w)
	if err != nil {
		return nil, err
	}
	return NewQuery(componentFilter), nil
}
