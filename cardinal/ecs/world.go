package ecs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"pkg.world.dev/world-engine/cardinal/ecs/message"

	"google.golang.org/protobuf/proto"

	shardv1 "pkg.world.dev/world-engine/rift/shard/v1"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"pkg.world.dev/world-engine/cardinal/ecs/component/metadata"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
	ecslog "pkg.world.dev/world-engine/cardinal/ecs/log"
	"pkg.world.dev/world-engine/cardinal/ecs/receipt"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/cardinal/ecs/store"
	"pkg.world.dev/world-engine/cardinal/events"
	"pkg.world.dev/world-engine/cardinal/shard"
	"pkg.world.dev/world-engine/chain/x/shard/types"
	"pkg.world.dev/world-engine/sign"
)

// Namespace is a unique identifier for a world.
type Namespace string

func (n Namespace) String() string {
	return string(n)
}

type World struct {
	namespace              Namespace
	nonceStore             storage.NonceStorage
	entityStore            store.IManager
	systems                []System
	systemLoggers          []*ecslog.Logger
	initSystem             System
	initSystemLogger       *ecslog.Logger
	systemNames            []string
	tick                   uint64
	nameToComponent        map[string]metadata.ComponentMetadata
	registeredComponents   []metadata.ComponentMetadata
	registeredMessages     []message.Message
	registeredQueries      []IQuery
	isComponentsRegistered bool
	isMessagesRegistered   bool
	stateIsLoaded          bool

	evmTxReceipts map[string]EVMTxReceipt

	txQueue *message.TxQueue

	receiptHistory *receipt.History

	chain shard.QueryAdapter
	// isRecovering indicates that the world is recovering from the DA layer.
	// this is used to prevent ticks from submitting duplicate transactions the DA layer.
	isRecovering bool

	Logger *ecslog.Logger

	endGameLoopCh     chan bool
	isGameLoopRunning atomic.Bool

	nextComponentID metadata.TypeID

	eventHub events.EventHub
}

var (
	ErrMessageRegistrationMustHappenOnce = errors.New("message registration must happen exactly 1 time")
	ErrStoreStateInvalid                 = errors.New("saved world state is not valid")
	ErrDuplicateMessageName              = errors.New("message names must be unique")
	ErrDuplicateQueryName                = errors.New("query names must be unique")
)

const (
	defaultReceiptHistorySize = 10
)

func (w *World) SetEventHub(eventHub events.EventHub) {
	w.eventHub = eventHub
}

func (w *World) EmitEvent(event *events.Event) {
	w.eventHub.EmitEvent(event)
}

func (w *World) FlushEvents() {
	w.eventHub.FlushEvents()
}

func (w *World) IsRecovering() bool {
	return w.isRecovering
}

func (w *World) Namespace() Namespace {
	return w.namespace
}

func (w *World) StoreManager() store.IManager {
	return w.entityStore
}

func (w *World) TickStore() store.TickStorage {
	return w.entityStore
}

func (w *World) GetTxQueueAmount() int {
	return w.txQueue.GetAmountOfTxs()
}

func (w *World) RegisterSystem(s System) {
	w.RegisterSystemWithName(s, "")
}

func (w *World) RegisterSystems(systems ...System) {
	for _, system := range systems {
		w.RegisterSystemWithName(system, "")
	}
}

func (w *World) RegisterSystemWithName(system System, functionName string) {
	if w.stateIsLoaded {
		panic("cannot register systems after loading game state")
	}
	if functionName == "" {
		functionName = filepath.Base(runtime.FuncForPC(reflect.ValueOf(system).Pointer()).Name())
	}
	sysLogger := w.Logger.CreateSystemLogger(functionName)
	w.systemLoggers = append(w.systemLoggers, &sysLogger)
	w.systemNames = append(w.systemNames, functionName)
	// appends registeredSystem into the member system list in world.
	w.systems = append(w.systems, system)
	w.checkDuplicateSystemName()
}

func (w *World) checkDuplicateSystemName() {
	mappedNames := make(map[string]int, len(w.systemNames))
	for _, sysName := range w.systemNames {
		if sysName != "" {
			mappedNames[sysName]++
			if mappedNames[sysName] > 1 {
				w.Logger.Warn().Msgf("duplicate system registered: %s", sysName)
			}
		}
	}
}

func (w *World) AddInitSystem(system System) {
	logger := w.Logger.CreateSystemLogger("InitSystem")
	w.initSystemLogger = &logger
	w.initSystem = system
}

func RegisterComponent[T metadata.Component](world *World) error {
	if world.stateIsLoaded {
		panic("cannot register components after loading game state")
	}
	var t T
	_, err := world.GetComponentByName(t.Name())
	if err == nil {
		return fmt.Errorf("component with name '%s' is already registered", t.Name())
	}
	c := metadata.NewComponentMetadata[T]()
	err = c.SetID(world.nextComponentID)
	if err != nil {
		return err
	}
	world.registeredComponents = append(world.registeredComponents, c)
	world.nextComponentID++
	world.nameToComponent[t.Name()] = c
	world.isComponentsRegistered = true
	return nil
}

func MustRegisterComponent[T metadata.Component](world *World) {
	err := RegisterComponent[T](world)
	if err != nil {
		panic(err)
	}
}

func (w *World) GetComponentByName(name string) (metadata.ComponentMetadata, error) {
	componentType, exists := w.nameToComponent[name]
	if !exists {
		return nil, fmt.Errorf("component with name %s not found. Must register component before using", name)
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
			return fmt.Errorf("duplicate query %q: %w", name, ErrDuplicateQueryName)
		}
		seenQueryNames[name] = struct{}{}
	}
	return nil
}

func (w *World) RegisterMessages(txs ...message.Message) error {
	if w.stateIsLoaded {
		panic("cannot register messages after loading game state")
	}
	if w.isMessagesRegistered {
		return ErrMessageRegistrationMustHappenOnce
	}
	w.isMessagesRegistered = true
	w.registerInternalMessages()
	w.registeredMessages = append(w.registeredMessages, txs...)

	seenTxNames := map[string]bool{}
	for i, t := range w.registeredMessages {
		name := t.Name()
		if seenTxNames[name] {
			return fmt.Errorf("duplicate tx %q: %w", name, ErrDuplicateMessageName)
		}
		seenTxNames[name] = true

		id := message.TypeID(i + 1)
		if err := t.SetID(id); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) registerInternalMessages() {
	w.registeredMessages = append(w.registeredMessages,
		CreatePersonaMsg,
		AuthorizePersonaAddressMsg,
	)
}

func (w *World) ListQueries() []IQuery {
	return w.registeredQueries
}

func (w *World) ListMessages() ([]message.Message, error) {
	if !w.isMessagesRegistered {
		return nil, errors.New("cannot list messages until message registration occurs")
	}
	return w.registeredMessages, nil
}

// NewWorld creates a new world.
func NewWorld(nonceStore storage.NonceStorage, entityStore store.IManager, opts ...Option) (*World, error) {
	logger := &ecslog.Logger{
		&log.Logger,
	}
	entityStore.InjectLogger(logger)
	w := &World{
		nonceStore:        nonceStore,
		entityStore:       entityStore,
		namespace:         "world",
		tick:              0,
		systems:           make([]System, 0),
		initSystem:        func(_ WorldContext) error { return nil },
		nameToComponent:   make(map[string]metadata.ComponentMetadata),
		txQueue:           message.NewTxQueue(),
		Logger:            logger,
		isGameLoopRunning: atomic.Bool{},
		endGameLoopCh:     make(chan bool),
		nextComponentID:   1,
		evmTxReceipts:     make(map[string]EVMTxReceipt),
	}
	w.isGameLoopRunning.Store(false)
	w.RegisterSystems(RegisterPersonaSystem, AuthorizePersonaAddressSystem)
	err := RegisterComponent[SignerComponent](w)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(w)
	}
	if w.receiptHistory == nil {
		w.receiptHistory = receipt.NewHistory(w.CurrentTick(), defaultReceiptHistorySize)
	}
	return w, nil
}

func (w *World) CurrentTick() uint64 {
	return w.tick
}

func (w *World) ReceiptHistorySize() uint64 {
	return w.receiptHistory.Size()
}

// Remove removes the given Entity from the world.
func (w *World) Remove(id entity.ID) error {
	return w.StoreManager().RemoveEntity(id)
}

// ConsumeEVMMsgResult consumes a tx result from an EVM originated Cardinal message.
// It will fetch the receipt from the map, and then delete ('consume') it from the map.
func (w *World) ConsumeEVMMsgResult(evmTxHash string) (EVMTxReceipt, bool) {
	r, ok := w.evmTxReceipts[evmTxHash]
	delete(w.evmTxReceipts, evmTxHash)
	return r, ok
}

// AddTransaction adds a transaction to the transaction queue. This should not be used directly.
// Instead, use a MessageType.AddToQueue to ensure type consistency. Returns the tick this transaction will be
// executed in.
func (w *World) AddTransaction(id message.TypeID, v any, sig *sign.Transaction) (
	tick uint64, txHash message.TxHash,
) {
	// TODO: There's no locking between getting the tick and adding the transaction, so there's no guarantee that this
	// transaction is actually added to the returned tick.
	tick = w.CurrentTick()
	txHash = w.txQueue.AddTransaction(id, v, sig)
	return tick, txHash
}

func (w *World) AddEVMTransaction(id message.TypeID, v any, sig *sign.Transaction, evmTxHash string) (
	tick uint64, txHash message.TxHash,
) {
	tick = w.CurrentTick()
	txHash = w.txQueue.AddEVMTransaction(id, v, sig, evmTxHash)
	return tick, txHash
}

const (
	warningThreshold = 100 * time.Millisecond
)

// Tick performs one game tick. This consists of taking a snapshot of all pending transactions, then calling
// each System in turn with the snapshot of transactions.
func (w *World) Tick(_ context.Context) error {
	nullSystemName := "No system is running."
	nameOfCurrentRunningSystem := nullSystemName
	defer func() {
		if panicValue := recover(); panicValue != nil {
			w.Logger.Error().Msgf("Tick: %d, Current running system: %s", w.tick, nameOfCurrentRunningSystem)
			panic(panicValue)
		}
	}()
	startTime := time.Now()
	tickAsString := strconv.FormatUint(w.tick, 10)
	w.Logger.Info().Str("tick", tickAsString).Msg("Tick started")
	if !w.stateIsLoaded {
		return errors.New("must load state before first tick")
	}
	txQueue := w.txQueue.CopyTransactions()

	if err := w.TickStore().StartNextTick(w.registeredMessages, txQueue); err != nil {
		return err
	}

	if w.CurrentTick() == 0 {
		wCtx := NewWorldContextForTick(w, txQueue, w.initSystemLogger)
		err := w.initSystem(wCtx)
		if err != nil {
			return err
		}
	}

	for i, sys := range w.systems {
		nameOfCurrentRunningSystem = w.systemNames[i]
		wCtx := NewWorldContextForTick(w, txQueue, w.systemLoggers[i])
		err := sys(wCtx)
		nameOfCurrentRunningSystem = nullSystemName
		if err != nil {
			return err
		}
	}
	if w.eventHub != nil {
		// world can be optionally loaded with or without an eventHub. If there is one, on every tick it must flush events.
		w.eventHub.FlushEvents()
	}
	if err := w.TickStore().FinalizeTick(); err != nil {
		return err
	}
	w.setEvmResults(txQueue.GetEVMTxs())
	w.tick++
	w.receiptHistory.NextTick()
	elapsedTime := time.Since(startTime)

	var logEvent *zerolog.Event
	message := "tick ended"
	if elapsedTime > warningThreshold {
		logEvent = w.Logger.Warn()
		message += fmt.Sprintf(", (warning: tick exceeded %dms)", warningThreshold.Milliseconds())
	} else {
		logEvent = w.Logger.Info()
	}
	logEvent.
		Int("tick_execution_time", int(elapsedTime.Milliseconds())).
		Str("tick", tickAsString).
		Msg(message)
	return nil
}

type EVMTxReceipt struct {
	ABIResult []byte
	Errs      []error
	EVMTxHash string
}

func (w *World) setEvmResults(txs []message.TxData) {
	// iterate over all EVM originated transactions
	for _, tx := range txs {
		// see if tx has a receipt. sometimes it won't because:
		// The system isn't using TxIterators && never explicitly called SetResult.
		rec, ok := w.receiptHistory.GetReceipt(tx.TxHash)
		if !ok {
			continue
		}
		evmRec := EVMTxReceipt{EVMTxHash: tx.EVMSourceTxHash}
		msg := w.getMessage(tx.MsgID)
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
		w.evmTxReceipts[evmRec.EVMTxHash] = evmRec
	}
}

func (w *World) StartGameLoop(ctx context.Context, tickStart <-chan time.Time, tickDone chan<- uint64) {
	w.Logger.Info().Msg("Game loop started")
	w.Logger.LogWorld(w, zerolog.InfoLevel)
	//todo: add links to docs related to each warning
	if !w.isComponentsRegistered {
		w.Logger.Warn().Msg("No components registered.")
	}
	if !w.isMessagesRegistered {
		w.Logger.Warn().Msg("No messages registered.")
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
					tickTheWorld() // immediately tick if queue is not empty to process all txs if queue is not empty.
				}
				break loop
			}
		}
		w.isGameLoopRunning.Store(false)
	}()
}

// WaitForNextTick waits for the next tick. It returns true if it successfully waited for the next tick.
// Returns false if we hit the timout threshold (5s).
//
// TODO: we probably want to make this timeout related to the actual tick hz.
// If a game has 10s ticks, this wouldn't work.
// ticket: https://linear.app/arguslabs/issue/WORLD-430/make-waitfornextticks-timeout-and-sleep-duration-a-factor-of-the
func (w *World) WaitForNextTick() bool {
	current := w.CurrentTick()
	timeout := time.After(5 * time.Second) //nolint:gomnd // fine for now.

	for {
		if w.CurrentTick() > current {
			return true
		}
		select {
		case <-timeout:
			return false // Timeout reached
		default:
			// TODO: sleep time should be a factor of the tick hz itself.
			time.Sleep(500 * time.Millisecond) //nolint:gomnd // fine for now.
		}
	}
}

func (w *World) IsGameLoopRunning() bool {
	return w.isGameLoopRunning.Load()
}

func (w *World) Shutdown() {
	if !w.IsGameLoopRunning() {
		return
	}
	w.endGameLoopCh <- true
	for w.IsGameLoopRunning() { // Block until loop stops.
		time.Sleep(100 * time.Millisecond) //nolint:gomnd // its ok.
	}
	if w.eventHub != nil {
		w.eventHub.ShutdownEventHub()
	}
}

// recoverGameState checks the status of the last game tick. If the tick was incomplete (indicating
// a problem when running one of the Systems), the snapshotted state is recovered and the pending
// transactions for the incomplete tick are returned. A nil recoveredTxs indicates there are no pending
// transactions that need to be processed because the last tick was successful.
func (w *World) recoverGameState() (recoveredTxs *message.TxQueue, err error) {
	start, end, err := w.TickStore().GetTickNumbers()
	if err != nil {
		return nil, err
	}
	w.tick = end
	// We successfully completed the last tick. Everything is fine
	if start == end {
		//nolint:nilnil // its ok.
		return nil, nil
	}
	return w.TickStore().Recover(w.registeredMessages)
}

func (w *World) LoadGameState() error {
	if w.stateIsLoaded {
		return errors.New("cannot load game state multiple times")
	}
	if !w.isMessagesRegistered {
		if err := w.RegisterMessages(); err != nil {
			return err
		}
	}

	if !w.isComponentsRegistered {
		err := RegisterComponent[SignerComponent](w)
		if err != nil {
			return err
		}
	}

	if err := w.entityStore.RegisterComponents(w.registeredComponents); err != nil {
		return err
	}

	w.stateIsLoaded = true
	recoveredTxs, err := w.recoverGameState()
	if err != nil {
		return err
	}

	if recoveredTxs != nil {
		w.txQueue = recoveredTxs
		if err = w.Tick(context.Background()); err != nil {
			return err
		}
	}
	w.receiptHistory.SetTick(w.tick)

	return nil
}

// RecoverFromChain will attempt to recover the state of the world based on historical transaction data.
// The function puts the world in a recovery state, and then queries all transaction batches under the world's
// namespace. The function will continuously ask the EVM base shard for batches, and run ticks for each batch returned.
//
//nolint:gocognit
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
	namespace := w.Namespace().String()
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
				if err = w.Tick(ctx); err != nil {
					return err
				}
				current = w.CurrentTick()
			}
			// we've now reached target. we need to inject the transactions and tick.
			transactions := tickedTxs.Txs
			for _, tx := range transactions {
				sp, err := w.decodeTransaction(tx.GameShardTransaction)
				if err != nil {
					return err
				}
				msg := w.getMessage(message.TypeID(tx.TxId))
				if msg == nil {
					return fmt.Errorf("error recovering tx with ID %d: tx id not found", tx.TxId)
				}
				v, err := msg.Decode(sp.Body)
				if err != nil {
					return err
				}
				w.AddTransaction(message.TypeID(tx.TxId), v, w.protoTransactionToGo(sp))
			}
			// run the tick for this batch
			if err = w.Tick(ctx); err != nil {
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

func (w *World) protoTransactionToGo(sp *shardv1.Transaction) *sign.Transaction {
	return &sign.Transaction{
		PersonaTag: sp.PersonaTag,
		Namespace:  sp.Namespace,
		Nonce:      sp.Nonce,
		Signature:  sp.Signature,
		Body:       sp.Body,
	}
}

func (w *World) decodeTransaction(bz []byte) (*shardv1.Transaction, error) {
	payload := new(shardv1.Transaction)
	err := proto.Unmarshal(bz, payload)
	return payload, err
}

// getMessage iterates over the all registered messages and returns the message.Message associated with the
// message.TypeID.
func (w *World) getMessage(id message.TypeID) message.Message {
	for _, msg := range w.registeredMessages {
		if id == msg.ID() {
			return msg
		}
	}
	return nil
}

func (w *World) GetNonce(signerAddress string) (uint64, error) {
	return w.nonceStore.GetNonce(signerAddress)
}

func (w *World) SetNonce(signerAddress string, nonce uint64) error {
	return w.nonceStore.SetNonce(signerAddress, nonce)
}

func (w *World) AddMessageError(id message.TxHash, err error) {
	w.receiptHistory.AddError(id, err)
}

func (w *World) SetMessageResult(id message.TxHash, a any) {
	w.receiptHistory.SetResult(id, a)
}

func (w *World) GetTransactionReceipt(id message.TxHash) (any, []error, bool) {
	rec, ok := w.receiptHistory.GetReceipt(id)
	if !ok {
		return nil, nil, false
	}
	return rec.Result, rec.Errs, true
}

func (w *World) GetTransactionReceiptsForTick(tick uint64) ([]receipt.Receipt, error) {
	return w.receiptHistory.GetReceiptsForTick(tick)
}

func (w *World) GetComponents() []metadata.ComponentMetadata {
	return w.registeredComponents
}

func (w *World) GetSystemNames() []string {
	return w.systemNames
}

func (w *World) InjectLogger(logger *ecslog.Logger) {
	w.Logger = logger
	w.StoreManager().InjectLogger(logger)
}

func (w *World) NewSearch(filter Filterable) (*Search, error) {
	componentFilter, err := filter.ConvertToComponentFilter(w)
	if err != nil {
		return nil, err
	}
	return NewSearch(componentFilter), nil
}

func GetRawJSONOfComponent(w *World, component metadata.ComponentMetadata, id entity.ID) (
	json.RawMessage, error) {
	return w.StoreManager().GetComponentForEntityInRawJSON(component, id)
}
